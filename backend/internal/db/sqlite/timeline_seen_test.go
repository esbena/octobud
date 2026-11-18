package sqlite

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/octobud-hq/octobud/backend/internal/db"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestTimelineSeenWritesCannotRegress(t *testing.T) {
	conn, err := db.OpenDatabase(filepath.Join(t.TempDir(), "timeline.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	goose.SetBaseFS(MigrationsFS)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.Up(conn, "migrations"))
	_, err = conn.ExecContext(t.Context(), `
		INSERT INTO repositories (id, user_id, name, full_name) VALUES (1, 'owner', 'r', 'o/r');
		INSERT INTO notifications (user_id, github_id, repository_id, subject_type, subject_title)
		VALUES ('owner', 'thread', 1, 'Issue', 'Topic');`)
	require.NoError(t, err)
	store := NewStore(conn)
	update := func(timestamp string) time.Time {
		t.Helper()
		result, err := store.UpdateTimelineLastSeenAt(t.Context(), "owner", "thread", timestamp)
		require.NoError(t, err)
		require.True(t, result.TimelineLastSeenAt.Valid)
		return result.TimelineLastSeenAt.Time
	}
	latest, err := time.Parse(time.RFC3339Nano, "2026-11-12T10:00:00.123456789Z")
	require.NoError(t, err)
	require.True(t, update(latest.Format(time.RFC3339Nano)).Equal(latest))
	for _, lateWrite := range []string{
		"2026-11-12T09:00:00Z",
		"2026-11-12T10:00:00.123456788Z",
		"2026-11-12T11:00:00.123456788+01:00",
		"2026-11-12T11:00:00.123456789+01:00",
	} {
		require.True(t, update(lateWrite).Equal(latest), lateWrite)
	}
	newer := latest.Add(time.Nanosecond)
	require.True(t, update(newer.Format(time.RFC3339Nano)).Equal(newer))

	start := make(chan struct{})
	errors := make(chan error, 20)
	var writes sync.WaitGroup
	for i := range 20 {
		writes.Add(1)
		go func() {
			defer writes.Done()
			<-start
			_, err := store.UpdateTimelineLastSeenAt(t.Context(), "owner", "thread",
				newer.Add(time.Duration(i)*time.Second).Format(time.RFC3339Nano))
			errors <- err
		}()
	}
	close(start)
	writes.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	final, err := store.GetNotificationByGithubID(t.Context(), "owner", "thread")
	require.NoError(t, err)
	require.True(t, final.TimelineLastSeenAt.Time.Equal(newer.Add(19*time.Second)))
	_, err = store.UpdateTimelineLastSeenAt(t.Context(), "another-owner", "thread", newer.Format(time.RFC3339Nano))
	require.ErrorIs(t, err, sql.ErrNoRows)
}

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
