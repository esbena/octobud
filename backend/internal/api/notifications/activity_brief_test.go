package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/octobud-hq/octobud/backend/internal/api/helpers"
	"github.com/octobud-hq/octobud/backend/internal/core/activitybrief"
	"github.com/octobud-hq/octobud/backend/internal/db"
	githubmocks "github.com/octobud-hq/octobud/backend/internal/github/mocks"
	"github.com/octobud-hq/octobud/backend/internal/github/types"
	"github.com/octobud-hq/octobud/backend/internal/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type briefModel func(context.Context, activitybrief.Snapshot) ([]activitybrief.Bullet, error)

func (f briefModel) Summarize(ctx context.Context, snapshot activitybrief.Snapshot) ([]activitybrief.Bullet, error) {
	return f(ctx, snapshot)
}

func briefRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/notifications/id/activity-brief", strings.NewReader(body))
	route := chi.NewRouteContext()
	route.URLParams.Add("githubID", "id")
	ctx := context.WithValue(request.Context(), chi.RouteCtxKey, route)
	return request.WithContext(helpers.ContextWithUserID(ctx, "owner"))
}

func TestActivityBriefValidatesRequestBeforeFetching(t *testing.T) {
	for _, body := range []string{
		`{}`, `{"since":"bad"}`, `{"since":"2999-01-01T00:00:00Z"}`,
		`{"since":"2026-01-01T00:00:00Z","other":1}`,
		`{"since":"2026-01-01T00:00:00Z"} {}`,
		`{"since":"2026-01-01T00:00:00Z","summarize":"yes"}`,
	} {
		t.Run(body, func(t *testing.T) {
			handler, _, _, auth := setupTestHandler(gomock.NewController(t))
			auth.EXPECT().GetUser(gomock.Any()).Return(&models.User{GithubUserID: "owner", GithubUsername: "reader"}, nil).AnyTimes()
			response := httptest.NewRecorder()
			handler.handleActivityBrief(response, briefRequest(body))
			require.Equal(t, http.StatusBadRequest, response.Code)
		})
	}
}

func TestActivityBriefRequiresOwnedNotification(t *testing.T) {
	handler, notifications, _, auth := setupTestHandler(gomock.NewController(t))
	auth.EXPECT().GetUser(gomock.Any()).Return(&models.User{GithubUserID: "owner", GithubUsername: "reader"}, nil).AnyTimes()
	notifications.EXPECT().GetByGithubID(gomock.Any(), "owner", "id").Return(db.Notification{}, sql.ErrNoRows)
	response := httptest.NewRecorder()
	handler.handleActivityBrief(response, briefRequest(`{"since":"2026-01-01T00:00:00Z","summarize":true}`))
	require.Equal(t, http.StatusNotFound, response.Code)
}

func TestActivityBriefRequiresAuthentication(t *testing.T) {
	handler, _, _, auth := setupTestHandler(gomock.NewController(t))
	auth.EXPECT().GetUser(gomock.Any()).Return(&models.User{}, nil)
	request := httptest.NewRequest(http.MethodPost, "/notifications/id/activity-brief", strings.NewReader(`{}`))
	response := httptest.NewRecorder()
	handler.handleActivityBrief(response, request)
	require.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestActivityBriefOnDemandAndErrors(t *testing.T) {
	for _, test := range []struct {
		name       string
		summarize  bool
		configured bool
		modelError bool
		status     int
	}{
		{"deterministic only", false, false, false, http.StatusOK},
		{"summarize", true, true, false, http.StatusOK},
		{"disabled", true, false, false, http.StatusServiceUnavailable},
		{"model failure", true, true, true, http.StatusBadGateway},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			handler, notifications, _, auth := setupTestHandler(ctrl)
			auth.EXPECT().GetUser(gomock.Any()).Return(&models.User{GithubUserID: "owner", GithubUsername: "reader"}, nil).AnyTimes()
			notifications.EXPECT().GetByGithubID(gomock.Any(), "owner", "id").Return(db.Notification{
				GithubID: "id", SubjectType: "Issue", SubjectTitle: "Original topic",
				SubjectURL: sql.NullString{String: "https://api.github.com/repos/o/r/issues/1", Valid: true},
			}, nil)
			client := githubmocks.NewMockClient(ctrl)
			handler.githubClient = client
			if test.status != http.StatusServiceUnavailable {
				at := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
				client.EXPECT().FetchTimeline(gomock.Any(), "o", "r", 1, 100, 1).Return(
					[]types.TimelineEvent{{ID: json.RawMessage("1"), Event: "closed", CreatedAt: &at}}, 1, nil)
			}
			if test.configured {
				handler.WithActivityBriefs(briefModel(func(_ context.Context, snapshot activitybrief.Snapshot) ([]activitybrief.Bullet, error) {
					require.Equal(t, "id", snapshot.NotificationID)
					require.Equal(t, "reader", snapshot.ReaderUsername)
					require.Len(t, snapshot.New, 1)
					if test.modelError {
						return nil, errors.New("synthetic model failure")
					}
					return []activitybrief.Bullet{{Text: "Issue closed.", EvidenceIDs: []string{snapshot.New[0].ID}}}, nil
				}))
				configureBriefTestSettings(t, handler, true)
			}
			body := `{"since":"2026-01-01T00:00:00Z","summarize":false}`
			if test.summarize {
				body = `{"since":"2026-01-01T00:00:00Z","summarize":true}`
			}
			response := httptest.NewRecorder()
			handler.handleActivityBrief(response, briefRequest(body))
			require.Equal(t, test.status, response.Code, response.Body.String())
			if test.status == http.StatusOK {
				var result activitybrief.Result
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
				require.Equal(t, "explicit_cutoff", result.BaselineSource)
				require.Equal(t, test.summarize, result.Summarized)
				require.Equal(t, []string{"closed"}, result.EventKinds)
			}
		})
	}
}
