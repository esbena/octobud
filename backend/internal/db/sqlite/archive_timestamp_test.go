package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/octobud-hq/octobud/backend/internal/db"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestMigrationLeavesUnknownArchiveTimesUnset(t *testing.T) {
	conn, err := db.OpenDatabase(filepath.Join(t.TempDir(), "archive.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	goose.SetBaseFS(MigrationsFS)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 11))

	_, err = conn.ExecContext(t.Context(), `
		INSERT INTO repositories (id, user_id, name, full_name) VALUES (1, 'owner', 'r', 'o/r');
		INSERT INTO notifications (
			user_id, github_id, repository_id, subject_type, subject_title, archived
		) VALUES
			('owner', 'archived', 1, 'Issue', 'Archived', 1),
			('owner', 'active', 1, 'Issue', 'Active', 0);`)
	require.NoError(t, err)

	require.NoError(t, goose.UpTo(conn, "migrations", 12))

	var archivedAt, activeAt sql.NullString
	require.NoError(t, conn.QueryRowContext(t.Context(),
		`SELECT last_archived_at FROM notifications WHERE github_id = 'archived'`,
	).Scan(&archivedAt))
	require.NoError(t, conn.QueryRowContext(t.Context(),
		`SELECT last_archived_at FROM notifications WHERE github_id = 'active'`,
	).Scan(&activeAt))

	require.False(t, archivedAt.Valid, "the previous archive time was never recorded")
	require.False(t, activeAt.Valid)
}

func TestArchiveTimestampMigrationCanBeReapplied(t *testing.T) {
	conn, err := db.OpenDatabase(filepath.Join(t.TempDir(), "archive.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	goose.SetBaseFS(MigrationsFS)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 12))

	_, err = conn.ExecContext(t.Context(), `
		INSERT INTO repositories (id, user_id, name, full_name) VALUES (1, 'owner', 'r', 'o/r');
		INSERT INTO notifications (
			user_id, github_id, repository_id, subject_type, subject_title, last_archived_at
		) VALUES ('owner', 'issue', 1, 'Issue', 'Issue', '2026-01-01T00:00:00Z');`)
	require.NoError(t, err)
	require.NoError(t, goose.Down(conn, "migrations"))

	var count int
	require.NoError(t, conn.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM pragma_table_info('notifications') WHERE name = 'last_archived_at'`,
	).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, goose.UpTo(conn, "migrations", 12))

	var archivedAt sql.NullString
	require.NoError(t, conn.QueryRowContext(t.Context(),
		`SELECT last_archived_at FROM notifications WHERE github_id = 'issue'`,
	).Scan(&archivedAt))
	require.False(t, archivedAt.Valid)
}

func TestArchiveTimestampsAreRecordedAndPreservedOnUnarchive(t *testing.T) {
	conn, err := db.OpenDatabase(filepath.Join(t.TempDir(), "archive.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	goose.SetBaseFS(MigrationsFS)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.Up(conn, "migrations"))
	_, err = conn.ExecContext(t.Context(), `
		INSERT INTO repositories (id, user_id, name, full_name) VALUES (1, 'owner', 'r', 'o/r');
		INSERT INTO notifications (user_id, github_id, repository_id, subject_type, subject_title)
		VALUES
			('owner', 'single', 1, 'Issue', 'Single'),
			('owner', 'bulk', 1, 'Issue', 'Bulk');`)
	require.NoError(t, err)

	store := NewStore(conn)
	archived, err := store.ArchiveNotification(t.Context(), "owner", "single")
	require.NoError(t, err)
	require.True(t, archived.LastArchivedAt.Valid)
	_, err = conn.ExecContext(t.Context(),
		`UPDATE notifications SET last_archived_at = '2000-01-01T00:00:00Z' WHERE github_id = 'single'`,
	)
	require.NoError(t, err)
	rearchived, err := store.ArchiveNotification(t.Context(), "owner", "single")
	require.NoError(t, err)
	require.Equal(t, "2000-01-01T00:00:00Z", rearchived.LastArchivedAt.Time.Format(time.RFC3339))

	unarchived, err := store.UnarchiveNotification(t.Context(), "owner", "single")
	require.NoError(t, err)
	require.True(t, unarchived.LastArchivedAt.Valid)
	require.True(t, rearchived.LastArchivedAt.Time.Equal(unarchived.LastArchivedAt.Time))

	updated, err := store.BulkArchiveNotifications(t.Context(), "owner", []string{"bulk"})
	require.NoError(t, err)
	require.EqualValues(t, 1, updated)
	bulk, err := store.GetNotificationByGithubID(t.Context(), "owner", "bulk")
	require.NoError(t, err)
	require.True(t, bulk.LastArchivedAt.Valid)
	_, err = conn.ExecContext(t.Context(),
		`UPDATE notifications SET last_archived_at = '2000-01-01T00:00:00Z' WHERE github_id = 'bulk'`,
	)
	require.NoError(t, err)
	_, err = store.BulkArchiveNotifications(t.Context(), "owner", []string{"bulk"})
	require.NoError(t, err)
	bulk, err = store.GetNotificationByGithubID(t.Context(), "owner", "bulk")
	require.NoError(t, err)
	require.Equal(t, "2000-01-01T00:00:00Z", bulk.LastArchivedAt.Time.Format(time.RFC3339))
}
