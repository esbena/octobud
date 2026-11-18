package activitybrief

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
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

func newCollectorClient(t *testing.T) *githubmocks.MockClient {
	t.Helper()
	client := githubmocks.NewMockClient(gomock.NewController(t))
	created := cutoff.Add(-24 * time.Hour)
	client.EXPECT().FetchOpeningPost(gomock.Any(), "o", "r", 1, gomock.Any()).Return(types.TimelineEvent{
		Event: "opened", ID: json.RawMessage(`"opening"`), CreatedAt: &created,
		Body: "Original post", User: &types.SimpleUser{Login: "author", Type: "User"},
		HTMLURL: "https://github.com/o/r/issues/1",
	}, nil).MaxTimes(1)
	return client
}

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

func TestCollectForReaderPreservesBodiesChronologyAndReviewThreads(t *testing.T) {
	client := newCollectorClient(t)
	old, newer := cutoff.Add(-time.Hour), cutoff.Add(time.Hour)
	parent, position := int64(10), 1
	body := strings.Repeat("\u00f8", 17000)
	client.EXPECT().FetchTimeline(gomock.Any(), "o", "r", 1, 100, 1).Return([]types.TimelineEvent{
		{Event: "reviewed", ID: json.RawMessage("1"), SubmittedAt: &old,
			User: &types.SimpleUser{Login: "Copilot", Type: "Bot"}, Body: body},
		{Event: "commented", ID: json.RawMessage("2"), CreatedAt: &old, UpdatedAt: &newer,
			User: &types.SimpleUser{Login: "reader", Type: "User"}, Body: body},
		{Event: "unfamiliar", ID: json.RawMessage("3"), CreatedAt: &newer},
		{Event: "committed", ID: json.RawMessage("4"), CreatedAt: &newer, Message: "Commit message"},
	}, 1, nil)
	client.EXPECT().FetchPullRequestComments(gomock.Any(), "o", "r", 1, 100, 1).Return([]types.PullRequestComment{
		{ID: parent, PullRequestReviewID: 1, CreatedAt: &old, Body: body, Path: "main.go", DiffHunk: "@@",
			User: types.SimpleUser{Login: "Copilot", Type: "Bot"}},
		{ID: 11, PullRequestReviewID: 1, InReplyToID: &parent, CreatedAt: &newer, Position: &position,
			User: types.SimpleUser{Login: "human", Type: "User"}, Body: "Correction"},
	}, nil)
	result, err := CollectForReader(t.Context(), client, sampleNotification("PullRequest"), cutoff, newer)
	require.NoError(t, err)
	byID := map[string]Evidence{}
	for _, events := range [][]Evidence{result.New, result.Context} {
		for _, event := range events {
			byID[event.ID] = event
		}
	}
	require.Equal(t, body, byID["timeline:reviewed:1"].Body)
	require.Equal(t, body, byID["review-comment:10"].Body)
	require.Equal(t, "bot", byID["review-comment:10"].ActorType)
	require.Equal(t, "human", byID["review-comment:11"].ActorType)
	require.Equal(t, "unknown", byID["timeline:unfamiliar:3"].ActorType)
	require.Equal(t, "review-comment:10", byID["review-comment:11"].RootID)
	require.Equal(t, "review-comment:10", byID["review-comment:11"].ParentID)
	require.Equal(t, "timeline:reviewed:1", byID["review-comment:11"].ReviewID)
	require.Equal(t, "main.go", byID["review-comment:10"].Path)
	require.Equal(t, `"main.go"`, byID["review-comment:10"].Detail)
	require.Equal(t, "@@", byID["review-comment:10"].DiffHunk)
	require.True(t, *byID["review-comment:10"].Outdated)
	require.False(t, *byID["review-comment:11"].Outdated)
	require.Equal(t, "edited", byID["timeline:commented:2"].Change)
	require.Equal(t, old, *byID["timeline:commented:2"].CreatedAt)
	require.Equal(t, newer, *byID["timeline:commented:2"].UpdatedAt)
	require.Equal(t, newer, *byID["timeline:commented:2"].At)
	require.Equal(t, old, *byID["timeline:reviewed:1"].CreatedAt)
	require.Equal(t, "context", byID["timeline:committed:4"].Change)
	require.Equal(t, "Commit message", byID["timeline:committed:4"].Message)
	require.Contains(t, result.Warnings, "Evidence has no deep link: review-comment:10")
}

func TestCollectForReaderClassifiesOpeningPostsWithTheSelectedCutoff(t *testing.T) {
	old, newer := cutoff.Add(-time.Hour), cutoff.Add(time.Hour)
	for _, test := range []struct {
		name    string
		kind    string
		created *time.Time
		edited  *time.Time
		change  string
	}{
		{"new issue", "Issue", &newer, nil, "created"},
		{"new pull request", "PullRequest", &newer, nil, "created"},
		{"new discussion", "Discussion", &newer, nil, "created"},
		{"edited body", "Issue", &old, &newer, "edited"},
		{"older post", "Issue", &old, nil, "context"},
		{"exact cutoff", "Issue", &cutoff, nil, "context"},
		{"missing creation time", "Issue", nil, nil, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := githubmocks.NewMockClient(gomock.NewController(t))
			if test.kind == "Discussion" {
				client.EXPECT().FetchDiscussionComments(gomock.Any(), "o", "r", 1, 100, "").
					Return(nil, false, "", nil)
			} else {
				client.EXPECT().FetchTimeline(gomock.Any(), "o", "r", 1, 100, 1).Return(nil, 1, nil)
				if test.kind == "PullRequest" {
					client.EXPECT().FetchPullRequestComments(gomock.Any(), "o", "r", 1, 100, 1).Return(nil, nil)
				}
			}
			client.EXPECT().FetchOpeningPost(gomock.Any(), "o", "r", 1, test.kind == "Discussion").
				Return(types.TimelineEvent{
					Event: "opened", ID: json.RawMessage(`"post"`), Body: "Opening **description**",
					CreatedAt: test.created, UpdatedAt: test.edited,
					User:    &types.SimpleUser{Login: "author", Type: "User"},
					HTMLURL: "https://github.com/o/r/issues/1",
				}, nil)
			result, err := CollectForReader(t.Context(), client, sampleNotification(test.kind), cutoff, newer)
			if test.change == "" {
				require.ErrorIs(t, err, ErrIncomplete)
				require.Empty(t, result)
				return
			}
			require.NoError(t, err)
			events := result.New
			if test.change == "context" {
				require.Empty(t, events)
				events = result.Context
			}
			require.Len(t, events, 1)
			require.Equal(t, "opened", events[0].Kind)
			require.Equal(t, test.change, events[0].Change)
			require.Equal(t, "Opening **description**", events[0].Body)
			require.Equal(t, "human", events[0].ActorType)
			require.Equal(t, "timeline:opened:post", events[0].RootID)
			require.Equal(t, test.created, events[0].CreatedAt)
			if test.change == "edited" {
				require.Equal(t, newer, *events[0].At)
			}
		})
	}
}

func TestCollectForReaderFetchesOlderDiscussionReplies(t *testing.T) {
	client := newCollectorClient(t)
	old, newer := cutoff.Add(-time.Hour), cutoff.Add(time.Hour)
	client.EXPECT().FetchDiscussionComments(gomock.Any(), "o", "r", 1, 100, "").Return(
		[]types.TimelineEvent{{
			Event: "commented", ID: json.RawMessage(`"parent"`), CreatedAt: &old,
			ReplyCount: 2, HasMoreReplies: true, RepliesCursor: "recent",
			User: &types.SimpleUser{Login: "bot", Type: "Bot"},
			Replies: []types.DiscussionReplyData{{ID: "child", Body: "Updated", CreatedAt: &old, UpdatedAt: &newer,
				Author: types.SimpleUser{Login: "human", Type: "User"}, HTMLURL: "https://github.com/o/r/discussions/1#reply"}},
		}}, false, "", nil)
	client.EXPECT().FetchDiscussionReplies(gomock.Any(), "parent", 100, "recent").Return(
		[]types.DiscussionReplyData{{ID: "older", CreatedAt: &old}}, false, "", nil)
	result, err := CollectForReader(t.Context(), client, sampleNotification("Discussion"), cutoff, newer)
	require.NoError(t, err)
	require.Len(t, result.New, 1)
	require.Len(t, result.Context, 3)
	reply := result.New[0]
	require.Equal(t, "reply:child", reply.ID)
	require.Equal(t, "timeline:commented:parent", reply.ParentID)
	require.Equal(t, reply.ParentID, reply.RootID)
	require.Equal(t, "human", reply.ActorType)
	require.Equal(t, "https://github.com/o/r/discussions/1#reply", reply.URL)
	require.NotContains(t, strings.Join(result.Warnings, "\n"), "truncated")
}

func TestCollectForReaderBoundsDiscussionReplyRequestsAcrossCommentsAndPages(t *testing.T) {
	for _, count := range []int{10, 11} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			client := newCollectorClient(t)
			events := make([]types.TimelineEvent, count)
			for i := range events {
				id := strconv.Itoa(i + 1)
				events[i] = types.TimelineEvent{
					Event: "commented", ID: json.RawMessage(`"` + id + `"`), CreatedAt: &cutoff,
					ReplyCount: 1, HasMoreReplies: true, RepliesCursor: "recent",
				}
				if i < 10 {
					client.EXPECT().FetchDiscussionReplies(gomock.Any(), id, 100, "recent").
						Return([]types.DiscussionReplyData{{ID: "reply-" + id, CreatedAt: &cutoff}}, false, "", nil)
				}
			}
			client.EXPECT().FetchDiscussionComments(gomock.Any(), "o", "r", 1, 100, "").
				Return(events[:5], true, "next", nil)
			client.EXPECT().FetchDiscussionComments(gomock.Any(), "o", "r", 1, 100, "next").
				Return(events[5:], false, "", nil)

			result, err := CollectForReader(t.Context(), client, sampleNotification("Discussion"), cutoff, cutoff)
			if count == 11 {
				require.ErrorIs(t, err, ErrIncomplete)
				require.ErrorContains(t, err, "discussion reply request limit")
				require.Empty(t, result)
			} else {
				require.NoError(t, err)
				require.Len(t, result.Context, 21)
			}
		})
	}
}

func TestCollectForReaderRejectsIncompleteEvidence(t *testing.T) {
	tests := []struct {
		name     string
		events   []types.TimelineEvent
		comments []types.PullRequestComment
	}{
		{"missing creation time", []types.TimelineEvent{{Event: "commented", ID: json.RawMessage("1")}}, nil},
		{"zero creation time", []types.TimelineEvent{{Event: "reviewed", ID: json.RawMessage("1"), SubmittedAt: &time.Time{}}}, nil},
		{"missing ID", []types.TimelineEvent{{Event: "commented", CreatedAt: &cutoff}}, nil},
		{"missing review ID", nil, []types.PullRequestComment{{ID: 1, CreatedAt: &cutoff}}},
		{"missing reply parent", []types.TimelineEvent{{Event: "reviewed", ID: json.RawMessage("7"), SubmittedAt: &cutoff}},
			[]types.PullRequestComment{{ID: 1, PullRequestReviewID: 7, InReplyToID: ptrInt64(2), CreatedAt: &cutoff}}},
		{"cyclic reply parent", []types.TimelineEvent{{Event: "reviewed", ID: json.RawMessage("7"), SubmittedAt: &cutoff}},
			[]types.PullRequestComment{{ID: 1, PullRequestReviewID: 7, InReplyToID: ptrInt64(1), CreatedAt: &cutoff}}},
		{"duplicate evidence", []types.TimelineEvent{
			{Event: "commented", ID: json.RawMessage("1"), CreatedAt: &cutoff},
			{Event: "commented", ID: json.RawMessage("1"), CreatedAt: &cutoff},
		}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newCollectorClient(t)
			client.EXPECT().FetchTimeline(gomock.Any(), "o", "r", 1, 100, 1).Return(tt.events, 1, nil)
			client.EXPECT().FetchPullRequestComments(gomock.Any(), "o", "r", 1, 100, 1).Return(tt.comments, nil).MaxTimes(1)
			result, err := CollectForReader(t.Context(), client, sampleNotification("PullRequest"), cutoff, cutoff)
			require.ErrorIs(t, err, ErrIncomplete)
			require.Empty(t, result)
		})
	}
}

func TestCollectForReaderUsesReviewCommentTimestampsWithoutTimelineReview(t *testing.T) {
	client := newCollectorClient(t)
	old, submitted, newer := cutoff.Add(-time.Hour), cutoff.Add(time.Hour), cutoff.Add(2*time.Hour)
	parent := int64(10)
	client.EXPECT().FetchTimeline(gomock.Any(), "o", "r", 1, 100, 1).Return(
		[]types.TimelineEvent{
			{Event: "reviewed", ID: json.RawMessage("7"), SubmittedAt: &submitted},
		}, 1, nil)
	client.EXPECT().FetchPullRequestComments(gomock.Any(), "o", "r", 1, 100, 1).Return(
		[]types.PullRequestComment{
			{ID: parent, PullRequestReviewID: 7, CreatedAt: &old, UpdatedAt: &submitted, Body: "Earlier comment"},
			{ID: 11, PullRequestReviewID: 8, InReplyToID: &parent, CreatedAt: &newer, UpdatedAt: &newer, Body: "New reply"},
		},
		nil,
	)

	result, err := CollectForReader(t.Context(), client, sampleNotification("PullRequest"), cutoff, newer)
	require.NoError(t, err)
	require.Len(t, result.New, 2)
	require.Len(t, result.Context, 2)
	newByID := map[string]Evidence{}
	for _, event := range result.New {
		newByID[event.ID] = event
	}
	reply := newByID["review-comment:11"]
	require.Equal(t, "created", reply.Change)
	require.Equal(t, "review-comment:10", reply.ParentID)
	require.Equal(t, reply.ParentID, reply.RootID)
	require.Equal(t, "timeline:reviewed:8", reply.ReviewID)
	require.Contains(t, newByID, "timeline:reviewed:7")
	require.Equal(t, "review-comment:10", result.Context[1].ID)
}

func ptrInt64(value int64) *int64 { return &value }

func TestCollectForReaderRejectsPaginationLimitsAndReplyGaps(t *testing.T) {
	t.Run("timeline bound", func(t *testing.T) {
		client := newCollectorClient(t)
		client.EXPECT().FetchTimeline(gomock.Any(), "o", "r", 1, 100, 1).Return(nil, 11, nil)
		_, err := CollectForReader(t.Context(), client, sampleNotification("Issue"), cutoff, cutoff)
		require.ErrorIs(t, err, ErrIncomplete)
	})
	for _, scenario := range []string{"missing cursor", "repeated cursor", "missing replies", "fetch failure"} {
		t.Run(scenario, func(t *testing.T) {
			client := newCollectorClient(t)
			event := types.TimelineEvent{Event: "commented", ID: json.RawMessage(`"parent"`),
				CreatedAt: &cutoff, ReplyCount: 2, HasMoreReplies: true}
			if scenario != "missing cursor" {
				event.RepliesCursor = "cursor"
				more := scenario == "repeated cursor"
				var fetchErr error
				if scenario == "fetch failure" {
					fetchErr = errors.New("denied")
				}
				client.EXPECT().FetchDiscussionReplies(gomock.Any(), "parent", 100, "cursor").
					Return(nil, more, "cursor", fetchErr)
			}
			client.EXPECT().FetchDiscussionComments(gomock.Any(), "o", "r", 1, 100, "").Return(
				[]types.TimelineEvent{event}, false, "", nil)
			result, err := CollectForReader(t.Context(), client, sampleNotification("Discussion"), cutoff, cutoff)
			if scenario == "fetch failure" {
				require.ErrorContains(t, err, "denied")
			} else {
				require.ErrorIs(t, err, ErrIncomplete)
			}
			require.Empty(t, result)
		})
	}
}

func TestCollectForReaderRequiresBaseline(t *testing.T) {
	result, err := CollectForReader(t.Context(), nil, sampleNotification("Issue"), time.Time{}, cutoff)
	require.ErrorIs(t, err, ErrInvalid)
	require.Empty(t, result)
}

func TestCollectForReaderInspectsOldDiscussionParentsAcrossPages(t *testing.T) {
	client := newCollectorClient(t)
	old, newer := cutoff.Add(-time.Hour), cutoff.Add(time.Hour)
	client.EXPECT().FetchDiscussionComments(gomock.Any(), "o", "r", 1, 100, "").
		Return(nil, true, "older", nil)
	client.EXPECT().FetchDiscussionComments(gomock.Any(), "o", "r", 1, 100, "older").
		Return([]types.TimelineEvent{{
			Event: "commented", ID: json.RawMessage(`"old-root"`), CreatedAt: &old, ReplyCount: 1,
			Replies: []types.DiscussionReplyData{{ID: "new-reply", CreatedAt: &newer}},
		}}, false, "", nil)
	result, err := CollectForReader(t.Context(), client, sampleNotification("Discussion"), cutoff, newer)
	require.NoError(t, err)
	require.Len(t, result.New, 1)
	require.Len(t, result.Context, 2)
	require.Equal(t, result.Context[1].ID, result.New[0].RootID)
}

func TestCollectForReaderRejectsDiscussionCommentCursorCycle(t *testing.T) {
	client := newCollectorClient(t)
	client.EXPECT().FetchDiscussionComments(gomock.Any(), "o", "r", 1, 100, "").
		Return(nil, true, "older", nil)
	client.EXPECT().FetchDiscussionComments(gomock.Any(), "o", "r", 1, 100, "older").
		Return(nil, true, "older", nil)
	_, err := CollectForReader(t.Context(), client, sampleNotification("Discussion"), cutoff, cutoff)
	require.ErrorIs(t, err, ErrIncomplete)
}

func TestCollectForReaderPaginatesReviewCommentsAndRejectsBound(t *testing.T) {
	for _, bounded := range []bool{false, true} {
		name := "complete"
		pages := 2
		if bounded {
			name, pages = "bound", maxPages
		}
		t.Run(name, func(t *testing.T) {
			client := newCollectorClient(t)
			old, newer := cutoff.Add(-time.Hour), cutoff.Add(time.Hour)
			client.EXPECT().FetchTimeline(gomock.Any(), "o", "r", 1, 100, 1).Return(
				[]types.TimelineEvent{{Event: "reviewed", ID: json.RawMessage("7"), SubmittedAt: &old}}, 1, nil)
			for page := 1; page <= pages; page++ {
				size := pageSize
				if !bounded && page == pages {
					size = 1
				}
				comments := make([]types.PullRequestComment, size)
				for i := range comments {
					id := int64((page-1)*pageSize + i + 1)
					comments[i] = types.PullRequestComment{ID: id, PullRequestReviewID: 7, CreatedAt: &old}
					if id != 1 {
						comments[i].InReplyToID = ptrInt64(1)
					}
				}
				if page == pages {
					comments[0].CreatedAt = &newer
				}
				client.EXPECT().FetchPullRequestComments(gomock.Any(), "o", "r", 1, 100, page).Return(comments, nil)
			}
			result, err := CollectForReader(t.Context(), client, sampleNotification("PullRequest"), cutoff, newer)
			if bounded {
				require.ErrorIs(t, err, ErrIncomplete)
				require.ErrorContains(t, err, "1,000-comment bound")
				require.Empty(t, result)
				return
			}
			require.NoError(t, err)
			require.Len(t, result.New, 1)
			require.Len(t, result.Context, 102)
			require.Equal(t, "review-comment:1", result.New[0].RootID)
		})
	}
}
