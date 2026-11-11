//go:build test && integration

package integration

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	"github.com/octobud-hq/octobud/backend/integration/client"
	"github.com/octobud-hq/octobud/backend/integration/fixtures"
	"github.com/octobud-hq/octobud/backend/integration/testserver"
	"github.com/octobud-hq/octobud/backend/internal/db"
	"github.com/octobud-hq/octobud/backend/internal/db/sqlite"
	"github.com/octobud-hq/octobud/backend/internal/models"
)

func TestViewSidebarVisibility(t *testing.T) {
	RunWithBackends(t, func(t *testing.T, ts *testserver.TestServer, c *client.Client) {
		repo := fixtures.NewRepository().Build(t, t.Context(), ts.Store, ts.UserID)
		fixtures.NewNotification(repo.ID).Build(t, t.Context(), ts.Store, ts.UserID)
		helper := createReferenceView(t, ts, "Helper", "in:inbox")
		require.True(t, helper.ShowInSidebar)
		require.Equal(t, int64(1), helper.UnreadCount)

		update := func(id string, body any) models.View {
			t.Helper()
			content := referenceRequest(
				t,
				ts,
				http.MethodPut,
				"/api/views/"+id,
				body,
				http.StatusOK,
			)
			var response struct {
				View models.View `json:"view"`
			}
			require.NoError(t, json.Unmarshal(content, &response))
			return response.View
		}
		helper = update(helper.ID, map[string]bool{"showInSidebar": false, "isDefault": true})
		require.False(t, helper.ShowInSidebar)
		require.True(t, helper.IsDefault)
		require.Zero(t, helper.UnreadCount)

		projection := createReferenceView(t, ts, "Projection", `in:"Helper"`)
		require.True(t, projection.ShowInSidebar)
		require.Equal(t, int64(1), projection.UnreadCount)
		require.Equal(t, int64(1), c.ListNotifications(t, `in:"Helper"`, 1, 100).Total)

		content := referenceRequest(t, ts, http.MethodGet, "/api/views", nil, http.StatusOK)
		var listed struct {
			Views []models.View `json:"views"`
		}
		require.NoError(t, json.Unmarshal(content, &listed))
		byID := make(map[string]models.View)
		for _, saved := range listed.Views {
			byID[saved.ID] = saved
		}
		require.Contains(t, byID, helper.ID)
		require.False(t, byID[helper.ID].ShowInSidebar)
		require.Zero(t, byID[helper.ID].UnreadCount)
		require.Equal(t, int64(1), byID[projection.ID].UnreadCount)

		helper = update(helper.ID, map[string]string{"description": "Reusable query"})
		require.False(t, helper.ShowInSidebar)
		require.True(t, helper.IsDefault)
		update(projection.ID, map[string]bool{"showInSidebar": false})
		update(helper.ID, map[string]string{"name": "Renamed helper"})
		stored, err := ts.Store.GetView(t.Context(), ts.UserID, projection.ID)
		require.NoError(t, err)
		require.False(t, stored.ShowInSidebar, "reference rewrites must preserve visibility")
		require.Equal(t, `in:"Renamed helper"`, stored.Query.String)
		require.Equal(t, int64(1), c.ListNotifications(t, `in:"Projection"`, 1, 100).Total)

		helper = update(helper.ID, map[string]bool{"showInSidebar": true})
		require.True(t, helper.ShowInSidebar)
		require.True(t, helper.IsDefault)
		require.Equal(t, int64(1), helper.UnreadCount)

		content = referenceRequest(t, ts, http.MethodPost, "/api/views", map[string]any{
			"name": "Hidden at creation", "query": "in:inbox", "showInSidebar": false,
		}, http.StatusCreated)
		var created struct {
			View models.View `json:"view"`
		}
		require.NoError(t, json.Unmarshal(content, &created))
		require.False(t, created.View.ShowInSidebar)
		require.Zero(t, created.View.UnreadCount)
	})
}

func TestViewSidebarVisibilityMigration(t *testing.T) {
	database, err := db.OpenDatabase(filepath.Join(t.TempDir(), "views.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	require.NoError(t, goose.SetDialect("sqlite3"))
	goose.SetBaseFS(sqlite.MigrationsFS)
	require.NoError(t, goose.UpTo(database, "migrations", 7))
	_, err = database.ExecContext(t.Context(), `INSERT INTO views (user_id, name, slug, query)
		VALUES ('owner', 'Existing', 'existing', 'in:inbox')`)
	require.NoError(t, err)

	require.NoError(t, goose.Up(database, "migrations"))
	views, err := db.NewStore(database).ListViews(t.Context(), "owner")
	require.NoError(t, err)
	require.Len(t, views, 1)
	require.True(t, views[0].ShowInSidebar)
	require.Equal(t, "in:inbox", views[0].Query.String)
}
