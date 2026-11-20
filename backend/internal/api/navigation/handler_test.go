// Copyright (C) 2025 Austin Beattie
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package navigation

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNavigationStreamSurvivesServerWriteTimeout(t *testing.T) {
	logger := zap.NewNop()
	broadcaster := NewBroadcaster(logger)
	router := chi.NewRouter()
	NewHandler(logger, broadcaster).Register(router)
	server := httptest.NewUnstartedServer(router)
	server.Config.WriteTimeout = 200 * time.Millisecond
	server.Start()
	defer server.Close()

	client := server.Client()
	client.Timeout = 3 * time.Second
	res, err := client.Get(server.URL + "/navigation-events")
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	lines := bufio.NewReader(res.Body)
	expectEvent := func(name string) {
		t.Helper()
		header, err := lines.ReadString('\n')
		require.NoError(t, err)
		require.Equal(t, "event: "+name+"\n", header)
		data, err := lines.ReadString('\n')
		require.NoError(t, err)
		require.Contains(t, data, "data: ")
		separator, err := lines.ReadString('\n')
		require.NoError(t, err)
		require.Equal(t, "\n", separator)
	}

	expectEvent("connected")
	time.Sleep(2 * server.Config.WriteTimeout)
	broadcaster.Broadcast(context.Background(), "/views/inbox")
	expectEvent("navigate")
}

func TestNavigationSettingsCloseStreamsAndPersist(t *testing.T) {
	dir := t.TempDir()
	logger := zap.NewNop()
	b, err := NewPersistentBroadcaster(logger, dir)
	require.NoError(t, err)
	require.False(t, b.NewTabMode())
	router := chi.NewRouter()
	NewHandler(logger, b).Register(router)
	server := httptest.NewServer(router)
	defer server.Close()
	client := server.Client()
	client.Timeout = 3 * time.Second

	responses := make([]*http.Response, 2)
	readers := make([]*bufio.Reader, 2)
	for i := range responses {
		responses[i], err = client.Get(server.URL + "/navigation-events")
		require.NoError(t, err)
		defer responses[i].Body.Close()
		readers[i] = bufio.NewReader(responses[i].Body)
		line, readErr := readers[i].ReadString('\n')
		require.NoError(t, readErr)
		require.Equal(t, "event: connected\n", line)
		_, readErr = readers[i].ReadString('\n')
		require.NoError(t, readErr)
		_, readErr = readers[i].ReadString('\n')
		require.NoError(t, readErr)
	}

	req, err := http.NewRequest(http.MethodPut, server.URL+"/navigation-settings", strings.NewReader(`{"newTab":true}`))
	require.NoError(t, err)
	res, err := client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, res.StatusCode)
	res.Body.Close()
	for _, reader := range readers {
		body, readErr := io.ReadAll(reader)
		require.NoError(t, readErr)
		require.Contains(t, string(body), "event: disabled\n")
	}
	res, err = client.Get(server.URL + "/navigation-events")
	require.NoError(t, err)
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	res.Body.Close()
	require.Contains(t, string(body), "event: disabled\n")

	reloaded, err := NewPersistentBroadcaster(logger, dir)
	require.NoError(t, err)
	require.True(t, reloaded.NewTabMode())
	info, err := os.Stat(filepath.Join(dir, "navigation-settings.json"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	require.NoError(t, reloaded.SetNewTabMode(false))
	reloaded, err = NewPersistentBroadcaster(logger, dir)
	require.NoError(t, err)
	require.False(t, reloaded.NewTabMode())
}

func TestNavigationSettingWriteFailureDoesNotChangeMode(t *testing.T) {
	dir := t.TempDir()
	b, err := NewPersistentBroadcaster(zap.NewNop(), dir)
	require.NoError(t, err)
	b.path = filepath.Join(dir, "missing", "settings.json")
	require.Error(t, b.SetNewTabMode(true))
	require.False(t, b.NewTabMode())
}

func TestInvalidNavigationSettingsAreRejected(t *testing.T) {
	b := NewBroadcaster(zap.NewNop())
	router := chi.NewRouter()
	NewHandler(zap.NewNop(), b).Register(router)
	req := httptest.NewRequest(http.MethodPut, "/navigation-settings", strings.NewReader(`{"newTab":"yes"}`))
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	require.Equal(t, http.StatusBadRequest, res.Code)
	require.False(t, b.NewTabMode())
}
