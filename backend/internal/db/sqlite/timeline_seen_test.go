package sqlite

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/octobud-hq/octobud/backend/internal/db"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestListNotificationsIncludesReadAndArchiveTimestamps(t *testing.T) {
	conn, err := db.OpenDatabase(filepath.Join(t.TempDir(), "timeline.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	goose.SetBaseFS(MigrationsFS)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.Up(conn, "migrations"))

	const timestamp = "2026-11-02T13:00:00Z"
	const archivedAt = "2026-11-01T13:00:00Z"
	_, err = conn.ExecContext(t.Context(), `
		INSERT INTO repositories (id, user_id, name, full_name) VALUES (1, 'owner', 'r', 'o/r');
		INSERT INTO notifications (
			user_id, github_id, repository_id, subject_type, subject_title,
			timeline_last_seen_at, last_archived_at
		) VALUES ('owner', 'thread', 1, 'PullRequest', 'Topic', ?, ?);`, timestamp, archivedAt)
	require.NoError(t, err)

	store := NewStore(conn)
	result, err := store.ListNotificationsFromQuery(t.Context(), "owner", db.NotificationQuery{
		Limit:          10,
		IncludeSubject: true,
	})
	require.NoError(t, err)
	require.Len(t, result.Notifications, 1)
	require.True(t, result.Notifications[0].TimelineLastSeenAt.Valid)
	require.Equal(
		t,
		timestamp,
		result.Notifications[0].TimelineLastSeenAt.Time.Format(time.RFC3339Nano),
	)
	require.True(t, result.Notifications[0].LastArchivedAt.Valid)
	require.Equal(
		t,
		archivedAt,
		result.Notifications[0].LastArchivedAt.Time.Format(time.RFC3339Nano),
	)
}
