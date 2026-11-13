package activitybrief

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResolveCutoffUsesLocalCalendarDaysAndCheckpoints(t *testing.T) {
	location, err := time.LoadLocation("America/Los_Angeles")
	require.NoError(t, err)
	now := time.Date(2026, 3, 8, 10, 30, 0, 0, time.UTC)

	today, err := ResolveCutoff("today", nil, nil, now, location)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 3, 8, 8, 0, 0, 0, time.UTC), today)

	yesterday, err := ResolveCutoff("since_yesterday", nil, nil, now, location)
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 3, 7, 8, 0, 0, 0, time.UTC), yesterday)

	readAt := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	readCutoff, err := ResolveCutoff("last_read", &readAt, nil, now, location)
	require.NoError(t, err)
	require.Equal(t, readAt, readCutoff)
	firstReadCutoff, err := ResolveCutoff("last_read", nil, nil, now, location)
	require.NoError(t, err)
	require.Equal(t, time.Unix(0, 0).UTC(), firstReadCutoff)
	archiveCutoff, err := ResolveCutoff("last_archive", nil, nil, now, location)
	require.NoError(t, err)
	require.Equal(t, time.Unix(0, 0).UTC(), archiveCutoff)
}

func TestRebaseSinceLastCommentKeepsNewActivityAndRelevantContext(t *testing.T) {
	old := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	newer := old.Add(time.Hour)
	snapshot := Snapshot{
		TotalEntries: 3,
		Since:        old.Add(-24 * time.Hour),
		New: []Evidence{
			{ID: "review-comment:reader", Kind: "review_comment", Actor: "Reader", Body: "My review", CreatedAt: &old, At: &old},
			{ID: "review-comment:reply", Kind: "review_comment", Actor: "author", Body: "I'll fix it", ParentID: "review-comment:reader", CreatedAt: &newer, At: &newer},
			{ID: "timeline:commented:later", Kind: "commented", Actor: "other", Body: "Another update", CreatedAt: &newer, At: &newer},
		},
	}

	result, err := RebaseSinceLastComment(snapshot, "reader")
	require.NoError(t, err)
	require.Equal(t, old, result.Since)
	require.Equal(t, 3, result.TotalEntries)
	require.Empty(t, result.BaselineMessage)
	require.Equal(t, []string{"review-comment:reply", "timeline:commented:later"}, []string{
		result.New[0].ID, result.New[1].ID,
	})
	require.Equal(t, []string{"review-comment:reader"}, []string{result.Context[0].ID})
}

func TestRebaseSinceLastCommentMakesFallbackExplicit(t *testing.T) {
	snapshot := Snapshot{New: []Evidence{{ID: "event", Kind: "closed"}}}
	result, err := RebaseSinceLastComment(snapshot, "")
	require.NoError(t, err)
	require.Contains(t, result.BaselineMessage, "Couldn't identify")
	require.Contains(t, result.Warnings, result.BaselineMessage)
}

func TestRebaseSinceLastCommentReclassifiesEditsAndKeepsCommitsAsContext(t *testing.T) {
	opened := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	commented := opened.Add(time.Hour)
	edited := commented.Add(time.Hour)
	snapshot := Snapshot{
		Since: time.Unix(0, 0).UTC(),
		New: []Evidence{
			{ID: "opening", Kind: "opened", Change: "created", Actor: "author",
				CreatedAt: &opened, UpdatedAt: &edited, At: &opened},
			{ID: "anchor", Kind: "commented", Change: "created", Actor: "reader",
				CreatedAt: &commented, At: &commented},
		},
		Context: []Evidence{{ID: "commit", Kind: "committed", Change: "context"}},
	}

	result, err := RebaseSinceLastComment(snapshot, "reader")
	require.NoError(t, err)
	require.Equal(t, commented, result.Since)
	require.Len(t, result.New, 1)
	require.Equal(t, "opening", result.New[0].ID)
	require.Equal(t, "edited", result.New[0].Change)
	require.Equal(t, edited, *result.New[0].At)
	require.Len(t, result.Context, 2)
	require.Equal(t, "committed", result.Context[0].Kind)
	require.Equal(t, "context", result.Context[0].Change)
}
