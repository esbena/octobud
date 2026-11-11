package user

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/octobud-hq/octobud/backend/internal/core/copilotsettings"
	"github.com/octobud-hq/octobud/backend/internal/db"
	dbmocks "github.com/octobud-hq/octobud/backend/internal/db/mocks"
	"github.com/octobud-hq/octobud/backend/internal/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCopilotSettingsDefaultOffRegardlessOfLegacyEnvironment(t *testing.T) {
	t.Setenv("OCTOBUD_ACTIVITY_BRIEFS", "true")
	ctrl := gomock.NewController(t)
	handler, auth := setupTestHandler(ctrl)
	auth.EXPECT().GetUser(gomock.Any()).Return(&models.User{GithubUserID: "owner"}, nil)
	store := dbmocks.NewMockStore(ctrl)
	store.EXPECT().GetUser(gomock.Any()).Return(db.User{}, nil)
	settings := copilotsettings.New(store, nil)
	t.Cleanup(settings.Close)
	handler.WithCopilotSettings(settings)
	response := httptest.NewRecorder()
	handler.handleGetCopilotSettings(
		response,
		httptest.NewRequest(http.MethodGet, "/", http.NoBody),
	)
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(
		t,
		`{"enabled":false,"activityHighlights":{"enabled":false,"defaultDays":3,"model":"gpt-6-luna"}}`,
		response.Body.String(),
	)
}

func TestCopilotSettingsRejectInvalidRequestsWithoutSaving(t *testing.T) {
	for _, body := range []string{
		`{"enabled":true,"token":"not-a-setting"}`,
		`{"activityHighlights":{"defaultDays":2}}`,
		`{"activityHighlights":{"model":""}}`,
		`{} {}`,
	} {
		t.Run(body, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			handler, auth := setupTestHandler(ctrl)
			auth.EXPECT().GetUser(gomock.Any()).Return(&models.User{GithubUserID: "owner"}, nil)
			settings := copilotsettings.New(dbmocks.NewMockStore(ctrl), nil)
			t.Cleanup(settings.Close)
			handler.WithCopilotSettings(settings)
			response := httptest.NewRecorder()
			handler.handleUpdateCopilotSettings(
				response,
				httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body)),
			)
			require.Equal(t, http.StatusBadRequest, response.Code)
		})
	}
}
