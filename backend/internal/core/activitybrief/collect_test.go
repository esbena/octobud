package activitybrief

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/octobud-hq/octobud/backend/internal/db"
	githubmocks "github.com/octobud-hq/octobud/backend/internal/github/mocks"
	"github.com/octobud-hq/octobud/backend/internal/github/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

var cutoff = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

func sampleNotification(kind string) db.Notification {
	return db.Notification{GithubID: "test", SubjectTitle: "Known original feature",
		SubjectType: kind, SubjectURL: sql.NullString{String: "https://api.github.com/repos/o/r/issues/1", Valid: true}}
}

func TestCollectSeparatesNewRepliesEditsAndOlderContext(t *testing.T) {
	client := githubmocks.NewMockClient(gomock.NewController(t))
	old, newer := cutoff.Add(-time.Hour), cutoff.Add(time.Hour)
	parent := int64(10)
	client.EXPECT().FetchTimeline(gomock.Any(), "o", "r", 1, 100, 1).Return([]types.TimelineEvent{
		{Event: "reviewed", ID: json.RawMessage("1"), SubmittedAt: &old, State: "APPROVED", Body: "old approval"},
		{Event: "committed", ID: json.RawMessage("2"), CreatedAt: &newer, Message: "Authored now does not mean pushed now"},
		{Event: "commented", ID: json.RawMessage("3"), CreatedAt: &old, UpdatedAt: &newer, Body: "Current body, not an edit diff"},
		{Event: "merged", ID: json.RawMessage("4"), CreatedAt: &newer},
		{Event: "commented", ID: json.RawMessage("5"), CreatedAt: &cutoff, Body: "exact boundary is old"},
	}, 1, nil)
	client.EXPECT().FetchPullRequestComments(gomock.Any(), "o", "r", 1, 100, 1).Return([]types.PullRequestComment{
		{ID: parent, CreatedAt: &old, Body: "old parent"},
		{ID: 11, InReplyToID: &parent, CreatedAt: &newer, Body: "Agreed"},
	}, nil)
	result, err := Collect(t.Context(), client, sampleNotification("PullRequest"), cutoff, newer)
	require.NoError(t, err)
	require.Len(t, result.New, 3)
	require.Len(t, result.Context, 4)
	byID := map[string]Evidence{}
	for _, e := range result.New {
		byID[e.ID] = e
	}
	require.Equal(t, "edited", byID["timeline:commented:3"].Change)
	require.Equal(t, "review-comment:10", byID["review-comment:11"].ParentID)
	require.Equal(t, "Agreed", byID["review-comment:11"].Body)
	require.NotContains(t, result.New, result.Context[0])
	built, err := Build(t.Context(), result, false, nil)
	require.NoError(t, err)
	require.Equal(t, []string{"commented", "merged", "review_comment"}, built.EventKinds)
	require.False(t, built.Summarized)
}

func TestCollectDiscussionTraversesOldParents(t *testing.T) {
	client := githubmocks.NewMockClient(gomock.NewController(t))
	old, newer := cutoff.Add(-time.Hour), cutoff.Add(time.Hour)
	gomock.InOrder(
		client.EXPECT().FetchDiscussionComments(gomock.Any(), "o", "r", 1, 100, "").Return(
			[]types.TimelineEvent{}, true, "older", nil),
		client.EXPECT().FetchDiscussionComments(gomock.Any(), "o", "r", 1, 100, "older").Return(
			[]types.TimelineEvent{{
				Event: "commented", ID: json.RawMessage(`"parent"`), CreatedAt: &old, HasMoreReplies: true,
				Replies: []types.DiscussionReplyData{{ID: "child", Body: "Suspected cause", CreatedAt: &newer}},
			}}, false, "", nil),
	)
	result, err := Collect(t.Context(), client, sampleNotification("Discussion"), cutoff, newer)
	require.NoError(t, err)
	require.Len(t, result.New, 1)
	require.Equal(t, "reply:child", result.New[0].ID)
	require.Equal(t, result.Context[0].ID, result.New[0].ParentID)
	require.Contains(t, strings.Join(result.Warnings, "\n"), "replies truncated")
}

func TestCollectWithholdsCopilotBodiesButKeepsHumanReplies(t *testing.T) {
	client := githubmocks.NewMockClient(gomock.NewController(t))
	old, newer := cutoff.Add(-time.Hour), cutoff.Add(time.Hour)
	parent := int64(10)
	client.EXPECT().FetchTimeline(gomock.Any(), "o", "r", 1, 100, 1).Return([]types.TimelineEvent{
		{Event: "reviewed", ID: json.RawMessage("1"), SubmittedAt: &newer, User: &types.SimpleUser{Login: "Copilot"}, Body: "Do not send these findings"},
	}, 1, nil)
	client.EXPECT().FetchPullRequestComments(gomock.Any(), "o", "r", 1, 100, 1).Return([]types.PullRequestComment{
		{ID: parent, CreatedAt: &old, User: types.SimpleUser{Login: "copilot[bot]"}, Body: "Do not send this root"},
		{ID: 11, InReplyToID: &parent, CreatedAt: &newer, User: types.SimpleUser{Login: "reader"}, Body: "Human correction remains"},
	}, nil)
	result, err := Collect(t.Context(), client, sampleNotification("PullRequest"), cutoff, newer)
	require.NoError(t, err)
	require.Len(t, result.Context, 1)
	require.Empty(t, result.Context[0].Body)
	require.Len(t, result.New, 2)
	bodies := map[string]string{}
	for _, event := range result.New {
		bodies[event.Actor] = event.Body
	}
	require.Empty(t, bodies["Copilot"])
	require.Equal(t, "Human correction remains", bodies["reader"])
}

func TestCollectSurfacesFetchFailures(t *testing.T) {
	client := githubmocks.NewMockClient(gomock.NewController(t))
	client.EXPECT().FetchTimeline(gomock.Any(), "o", "r", 1, 100, 1).Return(nil, 2, nil)
	client.EXPECT().FetchTimeline(gomock.Any(), "o", "r", 1, 100, 2).Return(nil, 0, errors.New("denied"))
	_, err := Collect(t.Context(), client, sampleNotification("Issue"), cutoff, cutoff)
	require.ErrorContains(t, err, "denied")
}

func TestCollectBoundsAndDisclosesPagination(t *testing.T) {
	client := githubmocks.NewMockClient(gomock.NewController(t))
	client.EXPECT().FetchTimeline(gomock.Any(), "o", "r", 1, 100, 1).Return(nil, 12, nil)
	for page := 12; page >= 3; page-- {
		client.EXPECT().FetchTimeline(gomock.Any(), "o", "r", 1, 100, page).Return(nil, 12, nil)
	}
	result, err := Collect(t.Context(), client, sampleNotification("Issue"), cutoff, cutoff)
	require.NoError(t, err)
	require.Contains(t, strings.Join(result.Warnings, "\n"), "newest 1,000")
}

func TestCollectRejectsInvalidOrUnsupportedRequests(t *testing.T) {
	_, err := Collect(t.Context(), nil, sampleNotification("Issue"), time.Time{}, cutoff)
	require.ErrorIs(t, err, ErrInvalid)
	_, err = Collect(t.Context(), nil, sampleNotification("Issue"), cutoff.Add(time.Second), cutoff)
	require.ErrorIs(t, err, ErrInvalid)
	_, err = Collect(t.Context(), nil, sampleNotification("CheckSuite"), cutoff, cutoff)
	require.ErrorIs(t, err, ErrUnsupported)
	_, err = Collect(t.Context(), nil, sampleNotification("Issue"), cutoff, cutoff)
	require.ErrorIs(t, err, ErrUnavailable)
}

func TestCollectPaginatesReviewRepliesAndDisclosesBodyTruncation(t *testing.T) {
	client := githubmocks.NewMockClient(gomock.NewController(t))
	old, newer := cutoff.Add(-time.Hour), cutoff.Add(time.Hour)
	client.EXPECT().FetchTimeline(gomock.Any(), "o", "r", 1, 100, 1).Return(nil, 1, nil)
	comments := make([]types.PullRequestComment, 100)
	for i := range comments {
		comments[i] = types.PullRequestComment{ID: int64(i + 1), CreatedAt: &old}
	}
	parent := int64(1)
	client.EXPECT().FetchPullRequestComments(gomock.Any(), "o", "r", 1, 100, 1).Return(comments, nil)
	client.EXPECT().FetchPullRequestComments(gomock.Any(), "o", "r", 1, 100, 2).Return(
		[]types.PullRequestComment{{ID: 101, InReplyToID: &parent, CreatedAt: &newer, Body: strings.Repeat("x", 17000)}}, nil)
	result, err := Collect(t.Context(), client, sampleNotification("PullRequest"), cutoff, newer)
	require.NoError(t, err)
	require.Len(t, result.New, 1)
	require.Equal(t, "review-comment:1", result.New[0].ParentID)
	require.LessOrEqual(t, len(result.New[0].Body), 16000)
	require.Contains(t, strings.Join(result.Warnings, "\n"), "Evidence body truncated")
}
