package user

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/octobud-hq/octobud/backend/internal/api/helpers"
	"github.com/octobud-hq/octobud/backend/internal/core/copilotsettings"
	"go.uber.org/zap"
)

func (h *Handler) WithCopilotSettings(settings *copilotsettings.Service) *Handler {
	h.copilotSettings = settings
	return h
}

func (h *Handler) handleGetCopilotSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := helpers.RequireUserID(r.Context(), w, h.authSvc); !ok {
		return
	}
	settings, err := h.copilotSettings.Get(r.Context())
	if err != nil {
		h.logger.Error("failed to load Copilot settings", zap.Error(err))
		helpers.WriteError(w, http.StatusInternalServerError, "Failed to load Copilot settings")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	helpers.WriteJSON(w, http.StatusOK, settings)
}

func (h *Handler) handleUpdateCopilotSettings(w http.ResponseWriter, r *http.Request) {
	if _, ok := helpers.RequireUserID(r.Context(), w, h.authSvc); !ok {
		return
	}
	settings := copilotsettings.Defaults()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&settings); err != nil {
		helpers.WriteError(w, http.StatusBadRequest, "Invalid Copilot settings")
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		helpers.WriteError(w, http.StatusBadRequest, "Unexpected trailing request data")
		return
	}
	if err := settings.Validate(); err != nil {
		helpers.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.copilotSettings.Update(r.Context(), settings); err != nil {
		h.logger.Error("failed to save Copilot settings", zap.Error(err))
		helpers.WriteError(w, http.StatusInternalServerError, "Failed to save Copilot settings")
		return
	}
	helpers.WriteJSON(w, http.StatusOK, settings)
}
