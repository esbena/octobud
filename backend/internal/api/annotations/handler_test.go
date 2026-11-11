package annotations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"

	authmocks "github.com/octobud-hq/octobud/backend/internal/core/auth/mocks"
	"github.com/octobud-hq/octobud/backend/internal/db"
	"github.com/octobud-hq/octobud/backend/internal/db/sqlite"
	"github.com/octobud-hq/octobud/backend/internal/models"
)

func TestAnnotationEndpoints(t *testing.T) {
	path := filepath.Join(t.TempDir(), "annotations.db")
	conn, err := db.OpenDatabase(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	goose.SetBaseFS(sqlite.MigrationsFS)
	t.Cleanup(func() { goose.SetBaseFS(nil) })
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.Up(conn, "migrations"))
	store := sqlite.NewStore(conn)
	userID := "alice"
	auth := authmocks.NewMockAuthService(gomock.NewController(t))
	auth.EXPECT().GetUser(gomock.Any()).DoAndReturn(func(context.Context) (*models.User, error) {
		return &models.User{GithubUserID: userID}, nil
	}).AnyTimes()
	router := chi.NewRouter()
	New(zap.NewNop(), store, auth).Register(router)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}

	w := request("GET", "/annotations", "")
	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{"annotations":[]}`, w.Body.String())
	code := "async ({ subject }) => [{text: '⚠️', color: '#ffffff', hoverText: subject?.body}]"
	input := map[string]any{
		"name": "Body inspection", "code": code,
		"viewIds": []string{"__inbox__", "custom-id", "__inbox__"},
	}
	body, err := json.Marshal(input)
	require.NoError(t, err)
	w = request("POST", "/annotations", string(body))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created struct {
		Annotation db.Annotation `json:"annotation"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.NotEmpty(t, created.Annotation.ID)
	require.True(t, created.Annotation.Enabled)
	require.Equal(t, code, created.Annotation.Code)
	require.Equal(t, []string{"__inbox__", "custom-id"}, created.Annotation.ViewIDs)
	id := created.Annotation.ID

	// Reopen the database to verify durable storage, not a process-local cache.
	persisted, err := db.OpenDatabase(path)
	require.NoError(t, err)
	items, err := sqlite.NewStore(persisted).ListAnnotations(context.Background(), userID)
	require.NoError(t, err)
	require.Equal(t, []db.Annotation{created.Annotation}, items)
	require.NoError(t, persisted.Close())

	userID = "bob"
	w = request("GET", "/annotations", "")
	require.JSONEq(t, `{"annotations":[]}`, w.Body.String())
	w = request("PUT", "/annotations/"+id, `{"name":"Other","code":"() => []","viewIds":[]}`)
	require.Equal(t, http.StatusNotFound, w.Code)
	w = request("DELETE", "/annotations/"+id, "")
	require.Equal(t, http.StatusNotFound, w.Code)

	userID = "alice"
	w = request("PUT", "/annotations/"+id, `{"name":"Updated","code":"() => []","viewIds":[]}`)
	require.Equal(t, http.StatusOK, w.Code)
	var updated struct {
		Annotation db.Annotation `json:"annotation"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	require.Equal(t, "Updated", updated.Annotation.Name)
	require.Empty(t, updated.Annotation.ViewIDs)
	w = request("GET", "/annotations", "")
	require.Contains(t, w.Body.String(), "Updated")
	w = request("PUT", "/annotations/"+id, `{"name":"Updated","code":"() => []","viewIds":["__inbox__"],"enabled":false}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	require.False(t, updated.Annotation.Enabled)
	require.Equal(t, []string{"__inbox__"}, updated.Annotation.ViewIDs)
	saved, err := store.ListAnnotations(context.Background(), userID)
	require.NoError(t, err)
	require.False(t, saved[0].Enabled)
	require.Equal(t, []string{"__inbox__"}, saved[0].ViewIDs)
	w = request("PUT", "/annotations/"+id, `{"name":"Updated","code":"() => []","viewIds":["__inbox__"],"enabled":true}`)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	require.True(t, updated.Annotation.Enabled)

	for _, invalid := range []string{
		`{`, `null`, `{}`, `{"name":"x"}`, `{"name":" ","code":"() => []"}`,
		`{"name":"x","code":" "}`, `{"name":"x","code":"() => []","viewIds":[" "]}`,
		`{"name":"x","code":"() => []","unknown":true}`,
		`{"name":"x","code":"() => []"} {}`,
	} {
		w = request("POST", "/annotations", invalid)
		require.Equal(t, http.StatusBadRequest, w.Code, invalid)
	}

	w = request("POST", "/annotations", `{"name":"x","code":"`+strings.Repeat("x", 1<<20)+`"}`)
	require.Equal(t, http.StatusBadRequest, w.Code)

	w = request("DELETE", "/annotations/"+id, "")
	require.Equal(t, http.StatusNoContent, w.Code)
	w = request("GET", "/annotations", "")
	require.JSONEq(t, `{"annotations":[]}`, w.Body.String())
	w = request("DELETE", "/annotations/"+id, "")
	require.Equal(t, http.StatusNotFound, w.Code)
	userID = ""
	w = request("GET", "/annotations", "")
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestExistingPreviewAnnotationsArePreserved(t *testing.T) {
	conn, err := db.OpenDatabase(filepath.Join(t.TempDir(), "migration.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	goose.SetBaseFS(sqlite.MigrationsFS)
	t.Cleanup(func() { goose.SetBaseFS(nil) })
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 12))
	_, err = conn.Exec(`CREATE TABLE annotations (
		id TEXT NOT NULL PRIMARY KEY, user_id TEXT NOT NULL, name TEXT NOT NULL,
		code TEXT NOT NULL, view_ids TEXT NOT NULL DEFAULT '[]',
		created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
		enabled INTEGER NOT NULL DEFAULT 1
	);
	CREATE INDEX annotations_user_id ON annotations (user_id);
	INSERT INTO annotations (id, user_id, name, code, view_ids, enabled)
		VALUES ('existing', 'alice', 'Existing', '() => []', '["__inbox__"]', 0)`)
	require.NoError(t, err)
	require.NoError(t, goose.Up(conn, "migrations"))
	items, err := sqlite.NewStore(conn).ListAnnotations(context.Background(), "alice")
	require.NoError(t, err)
	require.Equal(t, []db.Annotation{{
		ID: "existing", Name: "Existing", Code: "() => []", ViewIDs: []string{"__inbox__"}, Enabled: false,
	}}, items)
}

func TestAnnotationsUpgradeFromMain(t *testing.T) {
	conn, err := db.OpenDatabase(filepath.Join(t.TempDir(), "main.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	goose.SetBaseFS(sqlite.MigrationsFS)
	t.Cleanup(func() { goose.SetBaseFS(nil) })
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(conn, "migrations", 12))
	require.NoError(t, goose.Up(conn, "migrations"))
	annotation := db.Annotation{
		ID: "new", Name: "New", Code: "() => []", ViewIDs: []string{}, Enabled: true,
	}
	saved, err := sqlite.NewStore(conn).CreateAnnotation(context.Background(), "alice", annotation)
	require.NoError(t, err)
	require.Equal(t, annotation, saved)
}
