package copilotsettings

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/octobud-hq/octobud/backend/internal/db"
	dbmocks "github.com/octobud-hq/octobud/backend/internal/db/mocks"
	"github.com/octobud-hq/octobud/backend/internal/db/sqlite"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestPersistenceDefaultsAndDisabling(t *testing.T) {
	conn, err := db.OpenDatabase(filepath.Join(t.TempDir(), "settings.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	goose.SetBaseFS(sqlite.MigrationsFS)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.Up(conn, "migrations"))
	store := sqlite.NewStore(conn)
	_, err = store.CreateUser(t.Context())
	require.NoError(t, err)
	stopped := 0
	service := New(store, func() { stopped++ })
	t.Cleanup(service.Close)
	settings, err := service.Get(t.Context())
	require.NoError(t, err)
	require.Equal(t, Defaults(), settings)
	require.False(t, settings.HighlightsEnabled())

	settings.Enabled = true
	require.NoError(t, service.Update(t.Context(), settings))
	require.False(
		t, settings.HighlightsEnabled(), "master enablement alone must not enable features",
	)
	settings.ActivityHighlights = ActivityHighlights{
		Enabled: true, DefaultDays: 7, Model: "custom-model",
	}
	require.NoError(t, service.Update(t.Context(), settings))
	// A new service reads persisted preferences, not process-local state.
	reopened := New(store, nil)
	t.Cleanup(reopened.Close)
	loaded, err := reopened.Get(t.Context())
	require.NoError(t, err)
	require.Equal(t, settings, loaded)

	var running context.Context
	require.NoError(t, service.Use(t.Context(), func(s Settings, work context.Context) error {
		require.True(t, s.HighlightsEnabled())
		running = work
		return nil
	}))
	require.NoError(t, running.Err())
	settings.Enabled = false
	require.NoError(t, service.Update(t.Context(), settings))
	require.ErrorIs(t, running.Err(), context.Canceled)
	require.Equal(t, 2, stopped)
	loaded, err = reopened.Get(t.Context())
	require.NoError(t, err)
	require.False(t, loaded.HighlightsEnabled())
	require.True(
		t, loaded.ActivityHighlights.Enabled, "master switch preserves feature preferences",
	)

	settings.Enabled = true
	require.NoError(t, service.Update(t.Context(), settings))
	require.NoError(t, service.Use(t.Context(), func(_ Settings, work context.Context) error {
		running = work
		return work.Err()
	}))
	settings.ActivityHighlights.Enabled = false
	require.NoError(t, service.Update(t.Context(), settings))
	require.ErrorIs(t, running.Err(), context.Canceled)
}

func TestFailedSaveDoesNotCancelWork(t *testing.T) {
	store := dbmocks.NewMockStore(gomock.NewController(t))
	store.EXPECT().GetUser(gomock.Any()).Return(db.User{}, nil)
	store.EXPECT().
		UpdateUserCopilotSettings(gomock.Any(), gomock.Any()).
		Return(db.User{}, errors.New("disk full"))
	service := New(store, func() { t.Error("failed save canceled work") })
	t.Cleanup(service.Close)
	var work context.Context
	require.NoError(t, service.Use(t.Context(), func(_ Settings, ctx context.Context) error {
		work = ctx
		return nil
	}))
	require.ErrorContains(t, service.Update(t.Context(), Defaults()), "disk full")
	require.NoError(t, work.Err())
}

func TestInvalidPreferencesAreRejected(t *testing.T) {
	for _, days := range []int{0, 2, 31} {
		settings := Defaults()
		settings.ActivityHighlights.DefaultDays = days
		require.Error(t, settings.Validate())
	}
	for _, model := range []string{"", " ", " model"} {
		settings := Defaults()
		settings.ActivityHighlights.Model = model
		require.Error(t, settings.Validate())
	}
}
