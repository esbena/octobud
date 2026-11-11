//go:build test && integration

package integration

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/octobud-hq/octobud/backend/integration/client"
	"github.com/octobud-hq/octobud/backend/integration/testserver"
	"github.com/octobud-hq/octobud/backend/internal/db"
)

func TestViewPartialUpdates(t *testing.T) {
	RunWithBackends(t, func(t *testing.T, ts *testserver.TestServer, _ *client.Client) {
		expected, err := ts.Store.CreateView(t.Context(), ts.UserID, db.CreateViewParams{
			Name: "Reviews", Slug: "reviews", IsDefault: true,
			Description: sql.NullString{String: "Review requests", Valid: true},
			Icon:        sql.NullString{String: "star", Valid: true},
			Query:       sql.NullString{String: "in:inbox", Valid: true},
		})
		require.NoError(t, err)

		unchanged, err := ts.Store.UpdateView(
			t.Context(),
			ts.UserID,
			db.UpdateViewParams{ID: expected.ID},
		)
		require.NoError(t, err)
		require.Equal(t, expected, unchanged)

		expected.Query = sql.NullString{String: "in:archive", Valid: true}
		updated, err := ts.Store.UpdateView(t.Context(), ts.UserID, db.UpdateViewParams{
			ID: expected.ID, Query: expected.Query,
		})
		require.NoError(t, err)
		require.Equal(t, expected, updated)

		expected.IsDefault = false
		updated, err = ts.Store.UpdateView(t.Context(), ts.UserID, db.UpdateViewParams{
			ID: expected.ID, IsDefault: sql.NullBool{Bool: false, Valid: true},
		})
		require.NoError(t, err)
		require.Equal(t, expected, updated)
	})
}
