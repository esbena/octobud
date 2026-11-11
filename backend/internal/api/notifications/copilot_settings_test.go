package notifications

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/octobud-hq/octobud/backend/internal/core/activitybrief"
	"github.com/octobud-hq/octobud/backend/internal/core/copilotsettings"
	"github.com/octobud-hq/octobud/backend/internal/db"
	dbmocks "github.com/octobud-hq/octobud/backend/internal/db/mocks"
	"github.com/octobud-hq/octobud/backend/internal/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func configureBriefTestSettings(t *testing.T, h *Handler, enabled bool) *dbmocks.MockStore {
	t.Helper()
	store := dbmocks.NewMockStore(gomock.NewController(t))
	settings := copilotsettings.Defaults()
	settings.Enabled = enabled
	settings.ActivityHighlights.Enabled = enabled
	data, err := json.Marshal(settings)
	require.NoError(t, err)
	store.EXPECT().GetUser(gomock.Any()).Return(db.User{
		CopilotSettings: db.NullRawMessage{RawMessage: data, Valid: true},
	}, nil).AnyTimes()
	h.copilotSettings = copilotsettings.New(store, h.briefQueue.StopAll)
	t.Cleanup(func() {
		h.copilotSettings.Close()
		h.briefQueue.Close()
	})
	return store
}

func TestDisabledSettingsPreventBatchWork(t *testing.T) {
	handler, _, _, auth := setupTestHandler(gomock.NewController(t))
	handler.WithActivityBriefs(
		briefModel(func(context.Context, activitybrief.Snapshot) ([]activitybrief.Bullet, error) {
			t.Fatal("disabled feature invoked model")
			return nil, nil
		}),
	)
	configureBriefTestSettings(t, handler, false)
	auth.EXPECT().
		GetUser(gomock.Any()).
		Return(&models.User{GithubUserID: "owner", GithubUsername: "reader"}, nil).
		AnyTimes()
	response := httptest.NewRecorder()
	handler.handleActivityBriefBatch(response, briefRequest(`{"query":"in:inbox","days":3}`))
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.Empty(t, handler.briefQueue.Entries("owner"))
}

func TestDisablingSettingsCancelsRunningQueueWork(t *testing.T) {
	handler, _, _, _ := setupTestHandler(gomock.NewController(t))
	handler.WithActivityBriefs(briefModel(nil))
	store := configureBriefTestSettings(t, handler, true)
	started, stopped := make(chan struct{}), make(chan struct{})
	_, err := handler.briefQueue.Enqueue(activitybrief.Key{UserID: "owner", NotificationID: "one"},
		func(ctx context.Context, _ time.Time) (activitybrief.Result, error) {
			close(started)
			<-ctx.Done()
			close(stopped)
			return activitybrief.Result{}, ctx.Err()
		})
	require.NoError(t, err)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("job did not start")
	}
	store.EXPECT().UpdateUserCopilotSettings(gomock.Any(), gomock.Any()).Return(db.User{}, nil)
	require.NoError(t, handler.copilotSettings.Update(t.Context(), copilotsettings.Defaults()))
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("job was not canceled")
	}
	require.Equal(
		t,
		"cancelled", //nolint:misspell // Public queue status.
		handler.briefQueue.Entries("owner")[0].Status,
	)
}
