package notifications

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/octobud-hq/octobud/backend/internal/api/helpers"
	"github.com/octobud-hq/octobud/backend/internal/core/activitybrief"
	"go.uber.org/zap"
)

// handleWhatsNew is read-only and independent of model settings and the summary queue.
func (h *Handler) handleWhatsNew(w http.ResponseWriter, r *http.Request) {
	userID, ok := helpers.RequireUserID(r.Context(), w, h.authSvc)
	if !ok {
		return
	}
	var input struct {
		Since     time.Time `json:"since"`
		Selection string    `json:"selection"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		helpers.WriteError(w, http.StatusBadRequest, "expected since (RFC3339) and a cutoff selection")
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		helpers.WriteError(w, http.StatusBadRequest, "unexpected trailing request data")
		return
	}
	now := time.Now().UTC()
	if input.Since.IsZero() || input.Since.After(now) {
		helpers.WriteError(w, http.StatusBadRequest, "since must be a recorded timestamp no later than now")
		return
	}
	if input.Selection == "" {
		input.Selection = "last_read"
	}
	if !activitybrief.ValidSelection(input.Selection) {
		helpers.WriteError(w, http.StatusBadRequest, "selection is not a supported catch-up range")
		return
	}
	notification, err := h.notifications.GetByGithubID(r.Context(), userID, chi.URLParam(r, "githubID"))
	if errors.Is(err, sql.ErrNoRows) {
		helpers.WriteError(w, http.StatusNotFound, "notification not found")
		return
	}
	if err != nil {
		h.logger.Error("failed to load notification for reader", zap.Error(err))
		helpers.WriteError(w, http.StatusInternalServerError, "failed to load notification")
		return
	}
	snapshot, err := activitybrief.CollectForReader(
		r.Context(), h.githubClient, notification, input.Since, now,
	)
	if err != nil {
		h.activityBriefError(w, err)
		return
	}
	snapshot.Selection = input.Selection
	if input.Selection == "my_last_comment" {
		reader, readerErr := h.authSvc.GetUser(r.Context())
		if readerErr != nil || reader == nil || reader.GithubUserID != userID {
			if readerErr != nil {
				h.logger.Warn("failed to obtain reader identity", zap.Error(readerErr))
			}
		} else {
			snapshot.ReaderUsername = reader.GithubUsername
		}
		snapshot, err = activitybrief.RebaseSinceLastComment(snapshot, snapshot.ReaderUsername)
		if err != nil {
			h.activityBriefError(w, err)
			return
		}
	}
	helpers.WriteJSON(w, http.StatusOK, snapshot)
}
