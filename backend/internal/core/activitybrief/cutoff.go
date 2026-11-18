package activitybrief

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrNoBaseline = errors.New("the selected cutoff is unavailable for this notification")

func ValidSelection(selection string) bool {
	switch selection {
	case "today", "since_yesterday", "past_7_days", "my_last_comment", "last_read", "last_archive":
		return true
	default:
		return false
	}
}

func ResolveCutoff(
	selection string,
	lastReadAt, lastArchivedAt *time.Time,
	now time.Time,
	location *time.Location,
) (time.Time, error) {
	if location == nil {
		return time.Time{}, fmt.Errorf("%w: missing time zone", ErrInvalid)
	}
	switch selection {
	case "today", "since_yesterday":
		localNow := now.In(location)
		start := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location)
		if selection == "since_yesterday" {
			start = start.AddDate(0, 0, -1)
		}
		return start.UTC(), nil
	case "past_7_days":
		return now.Add(-7 * 24 * time.Hour).UTC(), nil
	case "last_read":
		if lastReadAt != nil {
			return lastReadAt.UTC(), nil
		}
		return time.Unix(0, 0).UTC(), nil
	case "last_archive":
		if lastArchivedAt != nil {
			return lastArchivedAt.UTC(), nil
		}
		return time.Unix(0, 0).UTC(), nil
	case "my_last_comment":
		return time.Unix(0, 0).UTC(), nil
	default:
		return time.Time{}, fmt.Errorf("%w: unsupported cutoff selection %q", ErrInvalid, selection)
	}
}

func RebaseSinceLastComment(snapshot Snapshot, username string) (Snapshot, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	evidence := append(append([]Evidence{}, snapshot.Context...), snapshot.New...)
	if username == "" {
		snapshot.BaselineMessage = "Couldn't identify your GitHub account; showing all available activity."
		snapshot.Warnings = append(snapshot.Warnings, snapshot.BaselineMessage)
		return snapshot, nil
	}
	var anchor *Evidence
	for index := range evidence {
		item := &evidence[index]
		if !isComment(item) || !strings.EqualFold(item.Actor, username) {
			continue
		}
		if timestampForAnchor(item) == nil {
			continue
		}
		if anchor == nil || timestampForAnchor(item).After(*timestampForAnchor(anchor)) {
			anchor = item
		}
	}
	if anchor == nil {
		snapshot.BaselineMessage = "No previous comment found; showing all available activity."
		snapshot.Warnings = append(snapshot.Warnings, snapshot.BaselineMessage)
		return snapshot, nil
	}
	anchorAt := timestampForAnchor(anchor)
	if anchorAt == nil {
		snapshot.BaselineMessage = "Couldn't determine your last comment time; showing all available activity."
		snapshot.Warnings = append(snapshot.Warnings, snapshot.BaselineMessage)
		return snapshot, nil
	}
	allByID := make(map[string]Evidence, len(evidence))
	for _, item := range evidence {
		allByID[item.ID] = item
	}
	newActivity := make([]Evidence, 0, len(evidence))
	contextIDs := map[string]bool{anchor.ID: true}
	for _, item := range evidence {
		if item.Kind == "committed" {
			contextIDs[item.ID] = true
			continue
		}
		if item.ID == anchor.ID || !latestEvidenceAtOrAfter(item, *anchorAt) {
			continue
		}
		if item.CreatedAt != nil && !item.CreatedAt.Before(*anchorAt) {
			item.Change, item.At = "created", item.CreatedAt
		} else if item.UpdatedAt != nil && !item.UpdatedAt.Before(*anchorAt) {
			item.Change, item.At = "edited", item.UpdatedAt
		}
		newActivity = append(newActivity, item)
		visited := map[string]bool{item.ID: true}
		for parentID := item.ParentID; parentID != "" && !visited[parentID]; {
			visited[parentID] = true
			contextIDs[parentID] = true
			parent, found := allByID[parentID]
			if !found {
				break
			}
			parentID = parent.ParentID
		}
	}
	newIDs := make(map[string]bool, len(newActivity))
	for _, item := range newActivity {
		newIDs[item.ID] = true
	}
	olderContext := make([]Evidence, 0, len(contextIDs))
	for _, item := range evidence {
		if contextIDs[item.ID] && !newIDs[item.ID] {
			olderContext = append(olderContext, item)
		}
	}
	snapshot.Since = anchorAt.UTC()
	snapshot.New = newActivity
	snapshot.Context = olderContext
	snapshot.BaselineMessage = ""
	return snapshot, nil
}

func isComment(item *Evidence) bool {
	switch item.Kind {
	case "commented", "comment", "review_comment", "discussion_reply":
		return true
	case "reviewed":
		return strings.TrimSpace(item.Body) != ""
	default:
		return false
	}
}

func timestampForAnchor(item *Evidence) *time.Time {
	if item.CreatedAt != nil && !item.CreatedAt.IsZero() {
		return item.CreatedAt
	}
	if item.At != nil && !item.At.IsZero() {
		return item.At
	}
	return nil
}

func latestEvidenceAtOrAfter(item Evidence, cutoff time.Time) bool {
	for _, timestamp := range []*time.Time{item.At, item.UpdatedAt, item.CreatedAt} {
		if timestamp == nil || timestamp.IsZero() {
			continue
		}
		if !timestamp.Before(cutoff) {
			return true
		}
	}
	return item.At == nil && item.UpdatedAt == nil && item.CreatedAt == nil
}
