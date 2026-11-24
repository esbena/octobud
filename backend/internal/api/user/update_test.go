// Copyright (C) 2025 Austin Beattie
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.

package user

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/octobud-hq/octobud/backend/internal/version"
)

type updateOSActions struct {
	installedVersion string
	installedErr     error
	versionCalls     int
	restarts         chan struct{}
}

func TestLinkedBuildMode(t *testing.T) {
	expected := os.Getenv("OCTOBUD_TEST_BUILD_MODE")
	if expected == "" {
		t.Skip("build mode not specified")
	}
	actions := &updateOSActions{installedVersion: "0.3.4", restarts: make(chan struct{}, 1)}
	handler := New(zap.NewNop(), nil).WithOSActionsService(actions)
	w := httptest.NewRecorder()
	handler.HandleGetInstalledVersion(w, httptest.NewRequest(http.MethodGet, "/api/user/installed-version", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var response VersionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	if expected == "development" {
		require.Equal(t, version.Get(), response.Version)
		w = httptest.NewRecorder()
		handler.HandleRestart(w, httptest.NewRequest(http.MethodPost, "/api/user/restart", nil))
		require.Equal(t, http.StatusForbidden, w.Code)
	} else {
		require.Equal(t, "0.3.4", response.Version)
	}
}

func (s *updateOSActions) GetInstalledAppVersion(context.Context) (string, error) {
	s.versionCalls++
	return s.installedVersion, s.installedErr
}

func (s *updateOSActions) RestartApp(context.Context) error {
	s.restarts <- struct{}{}
	return nil
}

func (s *updateOSActions) ActivateBrowserTab(context.Context, string, string, string) (bool, error) {
	return false, nil
}

func TestInstalledVersionBuildModes(t *testing.T) {
	oldDevelopment, oldVersion := version.Development, version.Version
	t.Cleanup(func() {
		version.Development, version.Version = oldDevelopment, oldVersion
	})
	version.Version = "local-test"

	for _, tt := range []struct {
		name        string
		development string
		installed   string
		err         error
		want        string
		calls       int
	}{
		{name: "development ignores installed app", development: "true", installed: "0.3.4", want: "local-test"},
		{name: "packaged build reads installed app", development: "false", installed: "0.3.4", want: "0.3.4", calls: 1},
		{name: "packaged build falls back on error", development: "false", err: errors.New("unavailable"), want: "local-test", calls: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			version.Development = tt.development
			actions := &updateOSActions{installedVersion: tt.installed, installedErr: tt.err}
			handler := New(zap.NewNop(), nil).WithOSActionsService(actions)
			w := httptest.NewRecorder()
			handler.HandleGetInstalledVersion(w, httptest.NewRequest(http.MethodGet, "/api/user/installed-version", nil))

			require.Equal(t, http.StatusOK, w.Code)
			var response VersionResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			require.Equal(t, tt.want, response.Version)
			require.Equal(t, tt.calls, actions.versionCalls)
		})
	}
}

func TestGetVersionReportsDevelopmentMode(t *testing.T) {
	oldDevelopment := version.Development
	t.Cleanup(func() { version.Development = oldDevelopment })

	for _, tt := range []struct {
		name        string
		development string
		want        bool
	}{
		{name: "development build", development: "true", want: true},
		{name: "packaged build", development: "false"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			version.Development = tt.development
			handler := New(zap.NewNop(), nil)
			w := httptest.NewRecorder()
			handler.HandleGetVersion(w, httptest.NewRequest(http.MethodGet, "/api/user/version", nil))

			require.Equal(t, http.StatusOK, w.Code)
			var response struct {
				Version       string `json:"version"`
				IsDevelopment bool   `json:"isDevelopment"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			require.Equal(t, version.Get(), response.Version)
			require.Equal(t, tt.want, response.IsDevelopment)
		})
	}
}

func TestRestartBuildModes(t *testing.T) {
	oldDevelopment := version.Development
	t.Cleanup(func() { version.Development = oldDevelopment })

	t.Run("development rejects restart", func(t *testing.T) {
		version.Development = "true"
		actions := &updateOSActions{restarts: make(chan struct{}, 1)}
		handler := New(zap.NewNop(), nil).WithOSActionsService(actions)
		w := httptest.NewRecorder()
		handler.HandleRestart(w, httptest.NewRequest(http.MethodPost, "/api/user/restart", nil))

		require.Equal(t, http.StatusForbidden, w.Code)
		select {
		case <-actions.restarts:
			t.Fatal("development build restarted the installed app")
		case <-time.After(200 * time.Millisecond):
		}
	})

	t.Run("packaged build restarts", func(t *testing.T) {
		version.Development = "false"
		actions := &updateOSActions{restarts: make(chan struct{}, 1)}
		handler := New(zap.NewNop(), nil).WithOSActionsService(actions)
		w := httptest.NewRecorder()
		handler.HandleRestart(w, httptest.NewRequest(http.MethodPost, "/api/user/restart", nil))

		require.Equal(t, http.StatusOK, w.Code)
		select {
		case <-actions.restarts:
		case <-time.After(time.Second):
			t.Fatal("packaged build did not restart")
		}
	})
}
