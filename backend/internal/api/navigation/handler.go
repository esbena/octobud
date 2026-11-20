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
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/octobud-hq/octobud/backend/internal/api/helpers"
	"go.uber.org/zap"
)

const eventWriteTimeout = 15 * time.Second

// Handler handles navigation-related HTTP routes.
type Handler struct {
	logger      *zap.Logger
	broadcaster *Broadcaster
}

// NewHandler creates a new navigation handler.
func NewHandler(logger *zap.Logger, broadcaster *Broadcaster) *Handler {
	return &Handler{
		logger:      logger,
		broadcaster: broadcaster,
	}
}

// Register registers navigation routes on the provided router.
func (h *Handler) Register(r chi.Router) {
	r.Get("/navigation-events", h.handleSSE)
	r.Get("/navigation-settings", h.handleGetSettings)
	r.Put("/navigation-settings", h.handlePutSettings)
}

func (h *Handler) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	helpers.WriteJSON(w, http.StatusOK, map[string]bool{"newTab": h.broadcaster.NewTabMode()})
}

func (h *Handler) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var settings struct {
		NewTab *bool `json:"newTab"`
	}
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil || settings.NewTab == nil {
		helpers.WriteError(w, http.StatusBadRequest, "newTab must be a boolean")
		return
	}
	if err := h.broadcaster.SetNewTabMode(*settings.NewTab); err != nil {
		h.logger.Error("Failed to save navigation settings", zap.Error(err))
		helpers.WriteError(w, http.StatusInternalServerError, "failed to save navigation settings")
		return
	}
	h.handleGetSettings(w, r)
}

// handleSSE handles Server-Sent Events connections for navigation commands.
func (h *Handler) handleSSE(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Subscribe to events
	eventCh, disabled, enabled := h.broadcaster.SubscribeIfEnabled()
	if !enabled {
		if err := sendEvent(w, http.NewResponseController(w), "disabled", nil); err != nil {
			h.logger.Warn("Failed to send navigation disabled event", zap.Error(err))
		}
		return
	}
	defer h.broadcaster.Unsubscribe(eventCh)
	response := http.NewResponseController(w)

	// Send initial connection message
	err := sendEvent(w, response, "connected", map[string]interface{}{
		"message": "Connected to navigation events",
	})
	if err != nil {
		h.logger.Error("Failed to send initial connected event", zap.Error(err))
		return
	}

	// Keep connection alive with periodic pings
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	ctx := r.Context()

	for {
		select {
		case <-disabled:
			if err := sendEvent(w, response, "disabled", nil); err != nil {
				h.logger.Warn("Failed to send navigation disabled event", zap.Error(err))
			}
			return
		case event := <-eventCh:
			// Check if context is done before trying to send
			select {
			case <-ctx.Done():
				return
			default:
			}

			err := sendEvent(w, response, "navigate", event)
			if err != nil {
				h.logger.Error(
					"Failed to send navigation event",
					zap.Error(err),
				)
				return
			}

		case <-ticker.C:
			// Send keepalive ping
			select {
			case <-ctx.Done():
				return
			default:
			}

			err := sendEvent(w, response, "ping", map[string]interface{}{
				"timestamp": time.Now().Unix(),
			})
			if err != nil {
				h.logger.Warn("Failed to send keepalive ping", zap.Error(err))
				return
			}

		case <-ctx.Done():
			return
		}
	}
}

func sendEvent(
	w http.ResponseWriter,
	response *http.ResponseController,
	eventType string,
	data any,
) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// Bound each write, not the idle time between events on this stream.
	if err := response.SetWriteDeadline(time.Now().Add(eventWriteTimeout)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, jsonData); err != nil {
		return err
	}
	if err := response.Flush(); err != nil {
		return err
	}
	return response.SetWriteDeadline(time.Time{})
}
