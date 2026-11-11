//go:build test && integration

package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	"github.com/octobud-hq/octobud/backend/integration/client"
	"github.com/octobud-hq/octobud/backend/integration/fixtures"
	"github.com/octobud-hq/octobud/backend/integration/testserver"
	"github.com/octobud-hq/octobud/backend/internal/core/view"
	"github.com/octobud-hq/octobud/backend/internal/db"
	"github.com/octobud-hq/octobud/backend/internal/db/sqlite"
	"github.com/octobud-hq/octobud/backend/internal/models"
	"github.com/octobud-hq/octobud/backend/internal/query"
)

func referenceRequest(
	t *testing.T,
	ts *testserver.TestServer,
	method, path string,
	body any,
	status int,
) []byte {
	t.Helper()
	data, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(
		t.Context(),
		method,
		ts.Server.URL+path,
		bytes.NewReader(data),
	)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := ts.Server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	content, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, status, resp.StatusCode, "%s", content)
	return content
}

func createReferenceView(t *testing.T, ts *testserver.TestServer, name, filter string) models.View {
	t.Helper()
	content := referenceRequest(t, ts, http.MethodPost, "/api/views",
		map[string]string{"name": name, "query": filter}, http.StatusCreated)
	var response struct {
		View models.View `json:"view"`
	}
	require.NoError(t, json.Unmarshal(content, &response))
	return response.View
}

func TestViewReferencesQueryConsumers(t *testing.T) {
	RunWithBackends(t, func(t *testing.T, ts *testserver.TestServer, c *client.Client) {
		ctx := context.Background()
		repo := fixtures.NewRepository().
			WithFullName("acme/project").
			Build(t, ctx, ts.Store, ts.UserID)
		fixtures.NewNotification(repo.ID).
			WithGithubID("noise").
			WithSubjectTitle("robot").
			Build(t, ctx, ts.Store, ts.UserID)
		fixtures.NewNotification(repo.ID).
			WithGithubID("human").
			WithSubjectTitle("review").
			Build(t, ctx, ts.Store, ts.UserID)
		fixtures.NewNotification(repo.ID).
			WithGithubID("read").
			WithSubjectTitle("review read").
			WithIsRead(true).
			Build(t, ctx, ts.Store, ts.UserID)
		fixtures.NewNotification(repo.ID).
			WithGithubID("archived").
			WithSubjectTitle("review").
			WithArchived(true).
			Build(t, ctx, ts.Store, ts.UserID)
		fixtures.NewNotification(repo.ID).
			WithGithubID("muted").
			WithSubjectTitle("robot").
			WithMuted(true).
			Build(t, ctx, ts.Store, ts.UserID)
		otherRepo := fixtures.NewRepository().
			WithFullName("acme/other").
			Build(t, ctx, ts.Store, ts.UserID)
		fixtures.NewNotification(otherRepo.ID).
			WithGithubID("other").
			WithSubjectTitle("review").
			Build(t, ctx, ts.Store, ts.UserID)
		noise := createReferenceView(t, ts, "Low priority", `title:robot`)
		active := createReferenceView(t, ts, "Active", `in:inbox,filtered`)
		actionable := createReferenceView(t, ts, "Actionable", `in:"Active" -in:"Low priority"`)
		joined := createReferenceView(t, ts, "Project", `repo:acme/project in:"Actionable"`)
		c.SetViewRepositoryDefault(t, active.ID, []int64{otherRepo.ID})

		result := c.ListNotifications(t, `in:"Project"`, 1, 1)
		require.Equal(t, int64(2), result.Total)
		require.Len(t, result.Notifications, 1)
		full := c.ListNotifications(t, `in:"Project"`, 1, 100)
		var ids []string
		for _, notification := range full.Notifications {
			ids = append(ids, notification.GithubID)
		}
		require.ElementsMatch(t, []string{"human", "read"}, ids)
		require.Equal(
			t,
			int64(3),
			c.ListNotifications(t, `in:"Project","Low priority"`, 1, 100).Total,
		)
		require.Equal(t, int64(3), c.ListNotifications(t,
			`title:review in:"Project" OR (in:"Low priority" AND title:robot)`, 1, 100).Total)
		require.Equal(t, int64(3), c.ListNotifications(t, `in:"Actionable"`, 1, 100).Total)
		require.Equal(t, int64(1), c.ListNotifications(t, `in:"Low priority"`, 1, 100).Total)
		require.Equal(
			t,
			int64(2),
			c.ListNotifications(t, `in:"Project" OR in:"Project"`, 1, 100).Total,
		)
		require.Equal(
			t,
			int64(0),
			c.ListNotificationsInRepositories(
				t,
				`in:"Project"`,
				[]int64{otherRepo.ID},
				1,
				100,
			).Total,
		)
		counts := c.ListRepositoryCounts(t, `in:"Project"`)
		require.Len(t, counts.Repositories, 1)
		require.Equal(t, int64(2), counts.Repositories[0].Total)
		require.Equal(t, int64(1), counts.Repositories[0].Unread)

		raw := referenceRequest(t, ts, http.MethodGet,
			"/api/notifications/poll?query="+url.QueryEscape(`in:"Project"`), nil, http.StatusOK)
		var poll struct {
			Total int64 `json:"total"`
		}
		require.NoError(t, json.Unmarshal(raw, &poll))
		require.Equal(t, int64(2), poll.Total)
		raw = referenceRequest(t, ts, http.MethodGet,
			"/api/notifications?query="+url.QueryEscape(`in:"Project"`), nil, http.StatusOK)
		var list struct {
			Notifications []models.Notification `json:"notifications"`
		}
		require.NoError(t, json.Unmarshal(raw, &list))
		require.Len(t, list.Notifications, 2)
		for _, notification := range list.Notifications {
			require.NotNil(t, notification.ActionHints)
			require.Contains(t, notification.ActionHints.DismissedOn, "archive")
		}

		views, err := view.NewService(ts.Store).ListViewsWithCounts(ctx, ts.UserID)
		require.NoError(t, err)
		badges := make(map[string]int64)
		for _, saved := range views {
			badges[saved.ID] = saved.UnreadCount
		}
		require.Equal(t, int64(1), badges[noise.ID])
		require.Equal(t, int64(2), badges[actionable.ID])
		require.Equal(t, int64(1), badges[joined.ID])

		bulk := c.BulkArchive(t, nil, `in:"Project"`)
		require.Equal(t, 2, bulk.Count)
		require.Equal(t, int64(0), c.ListNotifications(t, `in:"Project"`, 1, 100).Total)
		require.Equal(t, int64(1), c.ListNotifications(t, `in:"Actionable"`, 1, 100).Total)
		referenceRequest(t, ts, http.MethodPut, "/api/views/"+noise.ID,
			map[string]string{"query": "title:missing"}, http.StatusOK)
		require.Equal(t, int64(2), c.ListNotifications(t, `in:"Actionable"`, 1, 100).Total)
	})
}

func TestViewReferencesValidationAndRename(t *testing.T) {
	RunWithBackends(t, func(t *testing.T, ts *testserver.TestServer, _ *client.Client) {
		ctx := context.Background()
		referenceRequest(t, ts, http.MethodPost, "/api/views",
			map[string]string{"name": "Self", "query": `in:"Self"`}, http.StatusBadRequest)
		referenceRequest(t, ts, http.MethodPost, "/api/views",
			map[string]string{"name": "Unknown", "query": `in:"Missing"`}, http.StatusBadRequest)
		source := createReferenceView(t, ts, "Source", `in:inbox`)
		dependent := createReferenceView(
			t,
			ts,
			"Dependent",
			`in:"Source" OR (in:inbox,source AND title:"Source")`,
		)
		referenceRequest(
			t,
			ts,
			http.MethodPut,
			"/api/views/"+dependent.ID,
			map[string]any{
				"isDefault":   true,
				"description": "Keep this description",
				"icon":        "star",
			},
			http.StatusOK,
		)
		createReferenceView(t, ts, "Nested", `in:"Dependent"`)
		referenceRequest(t, ts, http.MethodPut, "/api/views/"+source.ID,
			map[string]string{"query": `in:"Nested"`}, http.StatusBadRequest)

		newName := `Révision "quoted"`
		referenceRequest(t, ts, http.MethodPut, "/api/views/"+source.ID,
			map[string]string{"name": newName}, http.StatusOK)
		stored, err := ts.Store.GetView(ctx, ts.UserID, dependent.ID)
		require.NoError(t, err)
		require.Equal(
			t,
			`in:"Révision \"quoted\"" OR (in:inbox,"Révision \"quoted\"" AND title:"Source")`,
			stored.Query.String,
		)
		require.Equal(t, "Dependent", stored.Name)
		require.Equal(t, "dependent", stored.Slug)
		require.Equal(t, "Keep this description", stored.Description.String)
		require.Equal(t, "star", stored.Icon.String)
		require.Equal(t, true, stored.IsDefault)
		names, err := query.ViewReferences(stored.Query.String)
		require.NoError(t, err)
		require.Equal(t, []string{newName, newName}, names)
		_, err = view.NewService(ts.Store).ListViewsWithCounts(ctx, ts.UserID)
		require.NoError(t, err)

		for _, suffix := range []string{"", "?force=true"} {
			body := referenceRequest(
				t,
				ts,
				http.MethodDelete,
				"/api/views/"+source.ID+suffix,
				nil,
				http.StatusConflict,
			)
			require.Contains(t, string(body), "Dependent")
			require.NotContains(t, string(body), "linkedRuleCount")
		}
		referenceRequest(t, ts, http.MethodPut, "/api/views/"+source.ID,
			map[string]string{"name": "anywhere"}, http.StatusBadRequest)
		referenceRequest(t, ts, http.MethodPut, "/api/views/"+dependent.ID,
			map[string]string{"query": "in:inbox"}, http.StatusOK)
		referenceRequest(
			t,
			ts,
			http.MethodDelete,
			"/api/views/"+source.ID,
			nil,
			http.StatusNoContent,
		)
	})
}

func TestViewReferencesRenameRollback(t *testing.T) {
	RunWithBackends(t, func(t *testing.T, ts *testserver.TestServer, _ *client.Client) {
		source := createReferenceView(t, ts, "Source", `in:inbox`)
		dependent := createReferenceView(t, ts, "Dependent", `in:"Source"`)
		_, err := ts.DB.ExecContext(
			t.Context(),
			`CREATE TRIGGER reject_reference_update BEFORE UPDATE OF query ON views
			WHEN OLD.name = 'Dependent' BEGIN SELECT RAISE(ABORT, 'dependency write blocked'); END`,
		)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, cleanupErr := ts.DB.ExecContext(
				context.Background(),
				"DROP TRIGGER reject_reference_update",
			)
			require.NoError(t, cleanupErr)
		})
		referenceRequest(t, ts, http.MethodPut, "/api/views/"+source.ID,
			map[string]string{"name": "Renamed"}, http.StatusInternalServerError)
		storedSource, err := ts.Store.GetView(t.Context(), ts.UserID, source.ID)
		require.NoError(t, err)
		require.Equal(t, "Source", storedSource.Name)
		storedDependent, err := ts.Store.GetView(t.Context(), ts.UserID, dependent.ID)
		require.NoError(t, err)
		require.Equal(t, `in:"Source"`, storedDependent.Query.String)
	})
}

func TestViewReferencesConcurrentEdit(t *testing.T) {
	database, err := db.OpenDatabase(filepath.Join(t.TempDir(), "views.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	require.NoError(t, goose.SetDialect("sqlite3"))
	goose.SetBaseFS(sqlite.MigrationsFS)
	require.NoError(t, goose.Up(database, "migrations"))
	store := db.NewStore(database)
	source, err := store.CreateView(t.Context(), "owner", db.CreateViewParams{
		Name: "Source", Slug: "source", Query: sql.NullString{String: "in:inbox", Valid: true},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	attempts := 0
	err = store.WithViewTransaction(ctx, func(transaction db.ViewStore) error {
		attempts++
		current, readErr := transaction.GetView(ctx, "owner", source.ID)
		if readErr != nil {
			return readErr
		}
		if attempts == 1 {
			require.Equal(t, "in:inbox", current.Query.String)
			// A separate connection commits after this transaction has read its snapshot.
			if _, updateErr := store.UpdateView(ctx, "owner", db.UpdateViewParams{
				ID: source.ID, Query: sql.NullString{String: "in:archive", Valid: true},
			}); updateErr != nil {
				return updateErr
			}
		} else {
			require.Equal(t, "in:archive", current.Query.String)
		}
		_, updateErr := transaction.UpdateView(ctx, "owner", db.UpdateViewParams{
			ID: source.ID, Name: sql.NullString{String: "Renamed", Valid: true},
			Slug: sql.NullString{String: "renamed", Valid: true},
		})
		return updateErr
	})
	require.NoError(t, err)
	require.Equal(t, 2, attempts)
	stored, err := store.GetView(t.Context(), "owner", source.ID)
	require.NoError(t, err)
	require.Equal(t, "Renamed", stored.Name)
	require.Equal(t, "in:archive", stored.Query.String)
}

func TestViewReferencesRejectRuleOwnedQueries(t *testing.T) {
	RunWithBackends(t, func(t *testing.T, ts *testserver.TestServer, _ *client.Client) {
		createReferenceView(t, ts, "Source", `in:inbox`)
		referenceRequest(t, ts, http.MethodPost, "/api/rules", map[string]any{
			"name": "Invalid", "query": `in:"Source"`, "actions": map[string]bool{"archive": true},
		}, http.StatusBadRequest)
		rule, err := ts.Store.CreateRule(t.Context(), ts.UserID, db.CreateRuleParams{
			Name: "Existing", Query: sql.NullString{String: "in:inbox", Valid: true}, Actions: []byte("{}"),
		})
		require.NoError(t, err)
		referenceRequest(t, ts, http.MethodPut, "/api/rules/"+rule.ID,
			map[string]string{"query": `-in:"Source"`}, http.StatusBadRequest)
	})
}

func TestViewReferencesNullableFieldsAndIsolation(t *testing.T) {
	RunWithBackends(t, func(t *testing.T, ts *testserver.TestServer, c *client.Client) {
		ctx := context.Background()
		repo := fixtures.NewRepository().Build(t, ctx, ts.Store, ts.UserID)
		fixtures.NewNotification(repo.ID).
			WithGithubID("null-author").
			Build(t, ctx, ts.Store, ts.UserID)
		createReferenceView(t, ts, "Bot", "author:bot")
		require.Equal(t, int64(1), c.ListNotifications(t, `in:inbox -in:"Bot"`, 1, 100).Total)
		_, err := ts.Store.CreateView(ctx, "another-user", db.CreateViewParams{
			Name: "Private", Slug: "private", Query: sql.NullString{String: "in:anywhere", Valid: true},
		})
		require.NoError(t, err)
		compiler := query.NewViewCompiler(ctx, ts.UserID, ts.Store)
		_, err = compiler.BuildQuery(`in:"Private"`, 1, 0, false)
		require.ErrorIs(t, err, query.ErrViewReferenceNotFound)
		fixtures.NewNotification(repo.ID).
			WithGithubID("other-user").
			Build(t, ctx, ts.Store, "another-user")
		createReferenceView(t, ts, "All saved", "in:anywhere")
		compiler = query.NewViewCompiler(ctx, ts.UserID, ts.Store)
		compiled, err := compiler.BuildQuery(`in:"All saved"`, 100, 0, false)
		require.NoError(t, err)
		result, err := ts.Store.ListNotificationsFromQuery(ctx, "another-user", compiled)
		require.NoError(t, err)
		require.Zero(t, result.Total, "the subquery remains scoped to its compiler's user")
	})
}
