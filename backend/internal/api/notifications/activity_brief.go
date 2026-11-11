package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/octobud-hq/octobud/backend/internal/api/helpers"
	"github.com/octobud-hq/octobud/backend/internal/copilotruntime"
	"github.com/octobud-hq/octobud/backend/internal/core/activitybrief"
	"github.com/octobud-hq/octobud/backend/internal/core/copilotsettings"
	"go.uber.org/zap"
)

func (h *Handler) StopActivityBriefs() {
	if h.copilotSettings != nil {
		h.copilotSettings.Close()
	}
	if h.briefQueue != nil {
		h.briefQueue.Close()
	}
}

func (h *Handler) ConfigureCopilotSettings() *copilotsettings.Service {
	h.briefQueue = activitybrief.NewQueue()
	h.briefFactory = activitybrief.NewCopilot
	h.copilotSettings = copilotsettings.New(h.store, func() {
		h.briefBatchMu.Lock()
		defer h.briefBatchMu.Unlock()
		if h.briefQueue != nil {
			h.briefQueue.StopAll()
		}
	})
	return h.copilotSettings
}

func (h *Handler) useBriefSettings(
	ctx context.Context,
	admit func(copilotsettings.Settings, context.Context) error,
) error {
	if h.copilotSettings == nil {
		return activitybrief.ErrUnavailable
	}
	return h.copilotSettings.Use(ctx, admit)
}

func (h *Handler) handleActivityBrief(w http.ResponseWriter, r *http.Request) {
	userID, ok := helpers.RequireUserID(r.Context(), w, h.authSvc)
	if !ok {
		return
	}
	var input struct {
		Since     time.Time `json:"since"`
		Summarize bool      `json:"summarize"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		helpers.WriteError(
			w,
			http.StatusBadRequest,
			"expected since (RFC3339) and optional summarize boolean",
		)
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		helpers.WriteError(w, http.StatusBadRequest, "unexpected trailing request data")
		return
	}
	now := time.Now().UTC()
	if input.Since.IsZero() || input.Since.After(now) {
		helpers.WriteError(
			w,
			http.StatusBadRequest,
			"since must be a nonzero timestamp no later than now",
		)
		return
	}
	// Scope all GitHub/model work to a notification owned by the authenticated user.
	notification, err := h.notifications.GetByGithubID(
		r.Context(),
		userID,
		chi.URLParam(r, "githubID"),
	)
	if errors.Is(err, sql.ErrNoRows) {
		helpers.WriteError(w, http.StatusNotFound, "notification not found")
		return
	}
	if err != nil {
		h.logger.Error("failed to load notification for activity brief", zap.Error(err))
		helpers.WriteError(w, http.StatusInternalServerError, "failed to load notification")
		return
	}
	var model activitybrief.Summarizer
	if input.Summarize {
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		var stop func() bool
		settingsErr := h.useBriefSettings(
			ctx,
			func(settings copilotsettings.Settings, work context.Context) error {
				if !settings.HighlightsEnabled() || h.briefFactory == nil {
					return activitybrief.ErrUnavailable
				}
				model = h.briefFactory(settings.ActivityHighlights.Model)
				stop = context.AfterFunc(work, cancel)
				return nil
			},
		)
		if settingsErr != nil {
			h.activityBriefError(w, settingsErr)
			return
		}
		defer stop()
		r = r.WithContext(ctx)
	}
	snapshot, err := activitybrief.Collect(
		r.Context(),
		h.githubClient,
		notification,
		input.Since,
		now,
	)
	if err != nil {
		h.activityBriefError(w, err)
		return
	}
	snapshot.Author = notification.AuthorLogin.String
	if input.Summarize {
		reader, readerErr := h.authSvc.GetUser(r.Context())
		if readerErr != nil || reader == nil || reader.GithubUsername == "" ||
			reader.GithubUserID != userID {
			if readerErr != nil {
				h.logger.Warn("failed to obtain summary reader identity", zap.Error(readerErr))
			}
			helpers.WriteError(
				w,
				http.StatusServiceUnavailable,
				"GitHub username is required to classify summary recipients.",
			)
			return
		}
		snapshot.ReaderUsername = reader.GithubUsername
	}
	result, err := activitybrief.Build(r.Context(), snapshot, input.Summarize, model)
	if err != nil {
		h.activityBriefError(w, err)
		return
	}
	helpers.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) activityBriefError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	switch {
	case errors.Is(err, activitybrief.ErrInvalid), errors.Is(err, activitybrief.ErrUnsupported):
		status = http.StatusBadRequest
	case errors.Is(err, activitybrief.ErrUnavailable),
		errors.Is(err, copilotruntime.ErrUnavailable):
		status = http.StatusServiceUnavailable
	case errors.Is(err, copilotruntime.ErrLimit):
		status = http.StatusRequestEntityTooLarge
	}
	h.logger.Warn("activity brief failed", zap.Error(err))
	helpers.WriteError(w, status, err.Error())
}
