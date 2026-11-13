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
	return collect(ctx, client, notification, since, now, false)
}

// CollectForReader preserves complete comment bodies and thread metadata.
// The opening post is classified alongside timeline events using creation and body-edit times.
// Incomplete source coverage is an error; callers must visibly fall back to the full view.
// Coverage is bounded to 1,000 timeline events, 1,000 review comments, 1,000 discussion
// comments, and 10 additional reply-page requests across the discussion; bounds are never silent.
// It still cannot reconstruct historical edits, commit arrival times, or discussion lifecycle events.
func CollectForReader(ctx context.Context, client githubinterfaces.Client, notification db.Notification, since, now time.Time) (Snapshot, error) {
	snapshot, err := collect(ctx, client, notification, since, now, true)
	if err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func collect(ctx context.Context, client githubinterfaces.Client, notification db.Notification, since, now time.Time, reader bool) (Snapshot, error) {
	snapshot := Snapshot{
		NotificationID: notification.GithubID, Title: notification.SubjectTitle,
		Since: since.UTC(), FetchedAt: now.UTC(), New: []Evidence{}, Context: []Evidence{},
		Warnings: []string{
			"Evidence is classified relative to the selected cutoff.",
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
	reviewSubmittedAt := map[string]time.Time{}
	replyPages := 0
	var evidenceErr error
	add := func(e Evidence, created, updated *time.Time, contextOnly bool) {
		if !reader && (strings.EqualFold(e.Actor, "Copilot") || strings.EqualFold(e.Actor, "copilot[bot]")) {
			e.Body = ""
		}
		if seen[e.ID] {
			if reader {
				evidenceErr = fmt.Errorf("%w: repeated evidence %s", ErrIncomplete, e.ID)
			}
			return
		}
		seen[e.ID] = true
		if reader {
			e.CreatedAt, e.UpdatedAt = created, updated
			if e.ActorType == "" {
				e.ActorType = "unknown"
			}
			if e.URL == "" {
				snapshot.Warnings = append(snapshot.Warnings, "Evidence has no deep link: "+e.ID)
			}
			if created == nil || created.IsZero() {
				switch e.Kind {
				case "opened", "commented", "reviewed", "review_comment", "discussion_reply":
					evidenceErr = fmt.Errorf("%w: missing creation timestamp for %s", ErrIncomplete, e.ID)
				default:
					snapshot.Warnings = append(snapshot.Warnings, "Evidence has no creation timestamp: "+e.ID)
				}
			}
		}
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
		if !reader && len(e.Body) > 16000 {
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
				if reader && (event.Event == "opened" || event.Event == "commented" || event.Event == "reviewed") {
					return fmt.Errorf("%w: missing %s ID", ErrIncomplete, event.Event)
				}
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
				if reader {
					e.ActorType = actorType(*event.Actor)
				}
			} else if event.User != nil {
				e.Actor = event.User.Login
				if reader {
					e.ActorType = actorType(*event.User)
				}
			}
			if reader {
				e.Message = event.Message
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
			if reader && event.Event == "reviewed" && created != nil {
				reviewSubmittedAt[id] = *created
			}
			// Commit timestamps describe Git history, not when a commit entered the PR.
			add(e, created, event.UpdatedAt, event.Event == "committed")
			if reader && event.HasMoreReplies {
				cursor := event.RepliesCursor
				cursors := map[string]bool{}
				for {
					if replyPages >= maxPages {
						return fmt.Errorf("%w: discussion reply request limit (%d pages)", ErrIncomplete, maxPages)
					}
					if cursor == "" || cursors[cursor] {
						return fmt.Errorf("%w: discussion reply pagination for %s", ErrIncomplete, e.ID)
					}
					cursors[cursor] = true
					replyPages++
					replies, more, next, err := client.FetchDiscussionReplies(ctx, id, pageSize, cursor)
					if err != nil {
						return fmt.Errorf("fetch reader discussion replies: %w", err)
					}
					event.Replies = append(event.Replies, replies...)
					if !more {
						break
					}
					cursor = next
				}
			} else if event.HasMoreReplies {
				snapshot.Warnings = append(snapshot.Warnings, "Discussion replies truncated: "+e.ID)
			}
			if reader && event.ReplyCount != len(event.Replies) {
				return fmt.Errorf("%w: discussion reply count for %s", ErrIncomplete, e.ID)
			}
			for _, reply := range event.Replies {
				if reader && reply.ID == "" {
					return fmt.Errorf("%w: discussion reply has no ID", ErrIncomplete)
				}
				r := Evidence{ID: "reply:" + reply.ID, Kind: "discussion_reply", ParentID: e.ID,
					Actor: reply.Author.Login, Body: reply.Body, URL: reply.HTMLURL}
				if reader {
					r.ActorType = actorType(reply.Author)
				}
				add(r, reply.CreatedAt, reply.UpdatedAt, false)
			}
		}
		return nil
	}
	if kind == "discussion" {
		cursor := ""
		cursors := map[string]bool{}
		for page := 1; page <= maxPages; page++ {
			cursors[cursor] = true
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
			if page == maxPages || next == "" || next == cursor || (reader && cursors[next]) {
				if reader {
					return snapshot, fmt.Errorf("%w: discussion comment pagination", ErrIncomplete)
				}
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
			if reader {
				return snapshot, fmt.Errorf("%w: timeline exceeds 1,000 events", ErrIncomplete)
			}
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
					if reader && (comment.ID <= 0 || comment.PullRequestReviewID <= 0) {
						return snapshot, fmt.Errorf("%w: review comment has no comment or review ID", ErrIncomplete)
					}
					pathDetail, err := json.Marshal(comment.Path)
					if err != nil {
						return snapshot, fmt.Errorf("encode review comment path: %w", err)
					}
					e := Evidence{ID: "review-comment:" + strconv.FormatInt(comment.ID, 10),
						Kind: "review_comment", Body: comment.Body, Actor: comment.User.Login,
						URL: comment.HTMLURL, Detail: string(pathDetail)}
					if comment.InReplyToID != nil {
						e.ParentID = "review-comment:" + strconv.FormatInt(*comment.InReplyToID, 10)
					}
					if reader {
						e.ActorType = actorType(comment.User)
						e.Path, e.DiffHunk = comment.Path, comment.DiffHunk
						outdated := comment.Position == nil
						e.Outdated = &outdated
						if comment.PullRequestReviewID != 0 {
							e.ReviewID = "timeline:reviewed:" + strconv.FormatInt(comment.PullRequestReviewID, 10)
						}
					}
					updated := comment.UpdatedAt
					if submittedAt, ok := reviewSubmittedAt[strconv.FormatInt(comment.PullRequestReviewID, 10)]; reader &&
						ok && updated != nil && updated.Equal(submittedAt) {
						// GitHub updates inline comments when their review is submitted.
						updated = nil
					}
					add(e, comment.CreatedAt, updated, false)
				}
				if len(comments) < pageSize {
					break
				}
				if page == maxPages {
					if reader {
						return snapshot, fmt.Errorf("%w: review comments reach 1,000-comment bound", ErrIncomplete)
					}
					snapshot.Warnings = append(snapshot.Warnings, "Review comment history truncated to the first 1,000 comments.")
				}
			}
		}
	}
	if reader {
		if evidenceErr != nil {
			return snapshot, evidenceErr
		}
		opening, err := client.FetchOpeningPost(ctx, subject.Owner, subject.Repo, subject.Number, kind == "discussion")
		if err != nil {
			return snapshot, fmt.Errorf("fetch reader opening post: %w", err)
		}
		if err := addTimeline([]types.TimelineEvent{opening}); err != nil {
			return snapshot, err
		}
		if evidenceErr != nil {
			return snapshot, evidenceErr
		}
		if err := linkReaderRoots(&snapshot); err != nil {
			return snapshot, err
		}
	}
	snapshot.TotalEntries = len(snapshot.New) + len(snapshot.Context)
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

func actorType(actor types.SimpleUser) string {
	if actor.Login == "" {
		return "unknown"
	}
	switch strings.ToLower(actor.Type) {
	case "bot":
		return "bot"
	case "user":
		return "human"
	default:
		return "unknown"
	}
}

func linkReaderRoots(snapshot *Snapshot) error {
	byID := map[string]*Evidence{}
	for _, events := range [][]Evidence{snapshot.New, snapshot.Context} {
		for i := range events {
			byID[events[i].ID] = &events[i]
		}
	}
	// ReviewID is optional: GitHub can return a review's comments without its timeline event.
	for _, e := range byID {
		root := e
		seen := map[string]bool{e.ID: true}
		for root.ParentID != "" {
			parent := byID[root.ParentID]
			if parent == nil || seen[root.ParentID] {
				return fmt.Errorf("%w: missing or cyclic parent %s for %s", ErrIncomplete, root.ParentID, e.ID)
			}
			seen[parent.ID] = true
			root = parent
		}
		e.RootID = root.ID
	}
	return nil
}
