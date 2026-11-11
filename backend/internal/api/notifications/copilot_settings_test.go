package notifications

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/octobud-hq/octobud/backend/internal/core/activitybrief"
	"github.com/octobud-hq/octobud/backend/internal/core/copilotsettings"
	"github.com/octobud-hq/octobud/backend/internal/db"
	dbmocks "github.com/octobud-hq/octobud/backend/internal/db/mocks"
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
