package notifications

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/octobud-hq/octobud/backend/internal/core/activitybrief"
	"github.com/octobud-hq/octobud/backend/internal/db"
	githubmocks "github.com/octobud-hq/octobud/backend/internal/github/mocks"
	"github.com/octobud-hq/octobud/backend/internal/github/types"
	"github.com/octobud-hq/octobud/backend/internal/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestWhatsNewRejectsMissingOrInvalidCutoff(t *testing.T) {
	for _, body := range []string{
		`{}`, `{"since":null}`, `{"since":"bad"}`, `{"since":"2999-01-01T00:00:00Z"}`,
		`{"since":"2026-01-01T00:00:00Z","summarize":true}`,
		`{"since":"2026-01-01T00:00:00Z"} {}`,
	} {
		t.Run(body, func(t *testing.T) {
			handler, _, _, auth := setupTestHandler(gomock.NewController(t))
			auth.EXPECT().GetUser(gomock.Any()).Return(&models.User{GithubUserID: "owner"}, nil)
			response := httptest.NewRecorder()
			handler.handleWhatsNew(response, briefRequest(body))
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
		})
	}
}

func TestWhatsNewRequiresOwnedNotification(t *testing.T) {
	handler, notifications, _, auth := setupTestHandler(gomock.NewController(t))
	auth.EXPECT().GetUser(gomock.Any()).Return(&models.User{GithubUserID: "owner"}, nil)
	notifications.EXPECT().GetByGithubID(gomock.Any(), "owner", "id").Return(db.Notification{}, sql.ErrNoRows)
	response := httptest.NewRecorder()
	handler.handleWhatsNew(response, briefRequest(`{"since":"2026-01-01T00:00:00Z"}`))
	require.Equal(t, http.StatusNotFound, response.Code)
}

func TestWhatsNewPreservesStrictCutoffWithoutModelOrSeenWrites(t *testing.T) {
	ctrl := gomock.NewController(t)
	handler, notifications, _, auth := setupTestHandler(ctrl)
	auth.EXPECT().GetUser(gomock.Any()).Return(&models.User{GithubUserID: "owner"}, nil)
	notifications.EXPECT().GetByGithubID(gomock.Any(), "owner", "id").Return(db.Notification{
		GithubID: "id", SubjectType: "Issue",
		SubjectURL: sql.NullString{String: "https://api.github.com/repos/o/r/issues/1", Valid: true},
	}, nil)
	client := githubmocks.NewMockClient(ctrl)
	expectBriefOpeningPost(client)
	handler.githubClient = client
	boundary := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fresh := boundary.Add(time.Second)
	client.EXPECT().FetchTimeline(gomock.Any(), "o", "r", 1, 100, 1).Return(
		[]types.TimelineEvent{
			{ID: json.RawMessage("1"), Event: "commented", CreatedAt: &boundary,
				Body: "Old comment", HTMLURL: "https://github.com/o/r/issues/1#issuecomment-1"},
			{ID: json.RawMessage("2"), Event: "commented", CreatedAt: &fresh,
				Body: "New comment", HTMLURL: "https://github.com/o/r/issues/1#issuecomment-2"},
		}, 1, nil)
	response := httptest.NewRecorder()
	handler.handleWhatsNew(response, briefRequest(`{"since":"2026-01-01T00:00:00Z"}`))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var result activitybrief.Snapshot
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.Equal(t, boundary, result.Since)
	require.Equal(t, 3, result.TotalEntries)
	require.Len(t, result.New, 1)
	require.Equal(t, "New comment", result.New[0].Body)
	require.Len(t, result.Context, 2)
	require.Equal(t, "Original post", result.Context[0].Body)
	require.Equal(t, "Old comment", result.Context[1].Body)
}

func TestWhatsNewIncludesTheOpeningPostWithoutComments(t *testing.T) {
	ctrl := gomock.NewController(t)
	handler, notifications, _, auth := setupTestHandler(ctrl)
	auth.EXPECT().GetUser(gomock.Any()).Return(&models.User{GithubUserID: "owner"}, nil)
	notifications.EXPECT().GetByGithubID(gomock.Any(), "owner", "id").Return(db.Notification{
		GithubID: "id", SubjectType: "Issue",
		SubjectURL: sql.NullString{String: "https://api.github.com/repos/o/r/issues/1", Valid: true},
	}, nil)
	client := githubmocks.NewMockClient(ctrl)
	handler.githubClient = client
	created := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	client.EXPECT().FetchTimeline(gomock.Any(), "o", "r", 1, 100, 1).Return(nil, 1, nil)
	client.EXPECT().FetchOpeningPost(gomock.Any(), "o", "r", 1, false).Return(types.TimelineEvent{
		Event: "opened", ID: json.RawMessage(`"opening"`), CreatedAt: &created,
		Body: "The issue's opening post", HTMLURL: "https://github.com/o/r/issues/1",
	}, nil)
	response := httptest.NewRecorder()
	handler.handleWhatsNew(response, briefRequest(`{"since":"1970-01-01T00:00:00Z","selection":"last_read"}`))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var result activitybrief.Snapshot
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.Len(t, result.New, 1)
	require.Equal(t, "opened", result.New[0].Kind)
	require.Equal(t, "The issue's opening post", result.New[0].Body)
}

func TestWhatsNewDoesNotReturnPartialHistoryOnCollectionFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	handler, notifications, _, auth := setupTestHandler(ctrl)
	auth.EXPECT().GetUser(gomock.Any()).Return(&models.User{GithubUserID: "owner"}, nil)
	notifications.EXPECT().GetByGithubID(gomock.Any(), "owner", "id").Return(db.Notification{
		GithubID: "id", SubjectType: "Issue",
		SubjectURL: sql.NullString{String: "https://api.github.com/repos/o/r/issues/1", Valid: true},
	}, nil)
	client := githubmocks.NewMockClient(ctrl)
	handler.githubClient = client
	client.EXPECT().FetchTimeline(gomock.Any(), "o", "r", 1, 100, 1).
		Return(nil, 1, activitybrief.ErrIncomplete)
	response := httptest.NewRecorder()
	handler.handleWhatsNew(response, briefRequest(`{"since":"2026-01-01T00:00:00Z"}`))
	require.Equal(t, http.StatusBadGateway, response.Code)
	require.NotContains(t, response.Body.String(), "newActivity")
}

func TestWhatsNewRequiresAuthentication(t *testing.T) {
	handler, _, _, auth := setupTestHandler(gomock.NewController(t))
	auth.EXPECT().GetUser(gomock.Any()).Return(&models.User{}, nil)
	response := httptest.NewRecorder()
	handler.handleWhatsNew(response, briefRequest(`{"since":"2026-01-01T00:00:00Z"}`))
	require.Equal(t, http.StatusUnauthorized, response.Code)
}
