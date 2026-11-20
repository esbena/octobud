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
	"net/http"
	"net/http/httptest"
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
