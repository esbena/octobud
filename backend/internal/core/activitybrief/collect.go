package activitybrief

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/octobud-hq/octobud/backend/internal/db"
	"github.com/octobud-hq/octobud/backend/internal/github"
	githubinterfaces "github.com/octobud-hq/octobud/backend/internal/github/interfaces"
	"github.com/octobud-hq/octobud/backend/internal/github/types"
)

const maxPages = 10
const pageSize = 100

// Collect uses raw GitHub events rather than the UI's filtered, shortened timeline.
// All pages in the bounded window are inspected: a new reply can have an old parent.
func Collect(ctx context.Context, client githubinterfaces.Client, notification db.Notification, since, now time.Time) (Snapshot, error) {
	snapshot := Snapshot{
		NotificationID: notification.GithubID, Title: notification.SubjectTitle,
		Since: since.UTC(), FetchedAt: now.UTC(), New: []Evidence{}, Context: []Evidence{},
		Warnings: []string{
			"Explicit cutoff supplied by the caller; not a recorded archive timestamp.",
			"No historical body or head snapshot: exact edits and code deltas cannot be reconstructed.",
		},
	}
	if since.IsZero() || since.After(now) {
		return snapshot, ErrInvalid
	}
	kind := strings.ToLower(strings.ReplaceAll(notification.SubjectType, "_", ""))
	if kind != "issue" && kind != "pullrequest" && kind != "pr" && kind != "discussion" {
		return snapshot, ErrUnsupported
	}
	if client == nil {
		return snapshot, ErrUnavailable
	}
	var raw json.RawMessage
	if notification.SubjectRaw.Valid {
		raw = notification.SubjectRaw.RawMessage
	}
	subject, err := github.ExtractSubjectInfo(notification.SubjectURL.String, raw)
	if err != nil {
		return snapshot, fmt.Errorf("extract activity brief subject: %w", err)
	}
	seen := map[string]bool{}
	add := func(e Evidence, created, updated *time.Time, contextOnly bool) {
		if strings.EqualFold(e.Actor, "Copilot") || strings.EqualFold(e.Actor, "copilot[bot]") {
			e.Body = ""
		}
		if seen[e.ID] {
			return
		}
		seen[e.ID] = true
		e.At = created
		e.Change = "context"
		if created == nil && updated == nil && !contextOnly {
			snapshot.Warnings = append(snapshot.Warnings, "Evidence has no activity timestamp: "+e.ID)
		}
		if !contextOnly && created != nil && created.After(since) {
			e.Change = "created"
		} else if !contextOnly && updated != nil && updated.After(since) {
			e.Change = "edited"
			e.At = updated
		}
		if len(e.Body) > 16000 {
			// Preserve UTF-8 while bounding prompt size.
			runes := []rune(e.Body)
			e.Body = string(runes[:min(4000, len(runes))])
			snapshot.Warnings = append(snapshot.Warnings, "Evidence body truncated: "+e.ID)
		}
		if e.Change == "context" {
			snapshot.Context = append(snapshot.Context, e)
		} else {
			snapshot.New = append(snapshot.New, e)
		}
	}
	addTimeline := func(events []types.TimelineEvent) error {
		for _, event := range events {
			id := strings.Trim(string(event.ID), `"`)
			if id == "" || id == "null" {
				encoded, err := json.Marshal(event)
				if err != nil {
					return fmt.Errorf("encode timeline evidence: %w", err)
				}
				id = fmt.Sprintf("%x", sha256.Sum256(encoded))
			}
			e := Evidence{ID: "timeline:" + event.Event + ":" + id, Kind: event.Event,
				Body: event.Body, State: event.State, URL: event.HTMLURL}
			if event.Actor != nil {
				e.Actor = event.Actor.Login
			} else if event.User != nil {
				e.Actor = event.User.Login
			}
			detail, err := json.Marshal(struct {
				Assignee  *types.SimpleUser      `json:"assignee,omitempty"`
				Reviewer  *types.SimpleUser      `json:"reviewer,omitempty"`
				Team      *types.SimpleTeam      `json:"team,omitempty"`
				Label     *types.TimelineLabel   `json:"label,omitempty"`
				Milestone *types.SimpleMilestone `json:"milestone,omitempty"`
				Rename    *types.TimelineRename  `json:"rename,omitempty"`
			}{event.Assignee, event.RequestedReviewer, event.RequestedTeam, event.Label, event.Milestone, event.Rename})
			if err != nil {
				return fmt.Errorf("encode timeline metadata: %w", err)
			}
			if string(detail) != "{}" {
				e.Detail = string(detail)
			}
			created := event.CreatedAt
			if event.SubmittedAt != nil {
				created = event.SubmittedAt
			}
			// Commit timestamps describe Git history, not when a commit entered the PR.
			add(e, created, event.UpdatedAt, event.Event == "committed")
			if event.HasMoreReplies {
				snapshot.Warnings = append(snapshot.Warnings, "Discussion replies truncated: "+e.ID)
			}
			for _, reply := range event.Replies {
				add(Evidence{ID: "reply:" + reply.ID, Kind: "discussion_reply", ParentID: e.ID,
					Actor: reply.Author.Login, Body: reply.Body, URL: reply.HTMLURL},
					reply.CreatedAt, reply.UpdatedAt, false)
			}
		}
		return nil
	}
	if kind == "discussion" {
		cursor := ""
		for page := 1; page <= maxPages; page++ {
			events, more, next, err := client.FetchDiscussionComments(ctx, subject.Owner, subject.Repo, subject.Number, pageSize, cursor)
			if err != nil {
				return snapshot, fmt.Errorf("fetch brief discussion comments: %w", err)
			}
			if err := addTimeline(events); err != nil {
				return snapshot, err
			}
			if !more {
				break
			}
			if page == maxPages || next == "" || next == cursor {
				snapshot.Warnings = append(snapshot.Warnings, "Discussion comment history truncated.")
				break
			}
			cursor = next
		}
		snapshot.Warnings = append(snapshot.Warnings, "Discussion coverage includes comments and available replies, not lifecycle events.")
	} else {
		first, last, err := client.FetchTimeline(ctx, subject.Owner, subject.Repo, subject.Number, pageSize, 1)
		if err != nil {
			return snapshot, fmt.Errorf("fetch brief timeline: %w", err)
		}
		start := max(1, last-maxPages+1)
		if start > 1 {
			snapshot.Warnings = append(snapshot.Warnings, "Timeline history truncated to the newest 1,000 events.")
		}
		for page := max(1, last); page >= start; page-- {
			events := first
			if page != 1 {
				events, _, err = client.FetchTimeline(ctx, subject.Owner, subject.Repo, subject.Number, pageSize, page)
				if err != nil {
					return snapshot, fmt.Errorf("fetch brief timeline page %d: %w", page, err)
				}
			}
			if err := addTimeline(events); err != nil {
				return snapshot, err
			}
		}
		if kind == "pullrequest" || kind == "pr" {
			for page := 1; page <= maxPages; page++ {
				comments, err := client.FetchPullRequestComments(ctx, subject.Owner, subject.Repo, subject.Number, pageSize, page)
				if err != nil {
					return snapshot, fmt.Errorf("fetch brief review comments: %w", err)
				}
				for _, comment := range comments {
					e := Evidence{ID: "review-comment:" + strconv.FormatInt(comment.ID, 10),
						Kind: "review_comment", Body: comment.Body, Actor: comment.User.Login,
						URL: comment.HTMLURL, Detail: comment.Path}
					if comment.InReplyToID != nil {
						e.ParentID = "review-comment:" + strconv.FormatInt(*comment.InReplyToID, 10)
					}
					add(e, comment.CreatedAt, comment.UpdatedAt, false)
				}
				if len(comments) < pageSize {
					break
				}
				if page == maxPages {
					snapshot.Warnings = append(snapshot.Warnings, "Review comment history truncated to the first 1,000 comments.")
				}
			}
		}
	}
	order := func(a, b Evidence) int {
		if a.At == nil && b.At == nil {
			return strings.Compare(a.ID, b.ID)
		}
		if a.At == nil {
			return -1
		}
		if b.At == nil {
			return 1
		}
		if c := a.At.Compare(*b.At); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	}
	slices.SortFunc(snapshot.New, order)
	slices.SortFunc(snapshot.Context, order)
	snapshot.Warnings = slices.Compact(snapshot.Warnings)
	return snapshot, nil
}
