package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/octobud-hq/octobud/backend/internal/api/helpers"
	"github.com/octobud-hq/octobud/backend/internal/core/activitybrief"
	"github.com/octobud-hq/octobud/backend/internal/core/copilotsettings"
	"github.com/octobud-hq/octobud/backend/internal/models"
	"go.uber.org/zap"
)

type briefStatusResponse struct {
	Enabled     bool                  `json:"enabled"`
	Entries     []activitybrief.Entry `json:"entries"`
	Queued      int                   `json:"queued"`
	Notice      string                `json:"notice,omitempty"`
	DefaultDays int                   `json:"defaultDays"`
}

func (h *Handler) briefStatus(
	userID string, settings copilotsettings.Settings,
) briefStatusResponse {
	entries := []activitybrief.Entry{}
	if h.briefQueue != nil {
		entries = h.briefQueue.Entries(userID)
	}
	return briefStatusResponse{
		Enabled: settings.HighlightsEnabled(), Entries: entries,
		DefaultDays: settings.ActivityHighlights.DefaultDays,
	}
}

func (h *Handler) handleActivityBriefStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := helpers.RequireUserID(r.Context(), w, h.authSvc)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	settings := copilotsettings.Defaults()
	if h.copilotSettings != nil {
		var err error
		settings, err = h.copilotSettings.Get(r.Context())
		if err != nil {
			h.logger.Error("failed to load Copilot settings", zap.Error(err))
			helpers.WriteError(w, http.StatusInternalServerError, "Failed to load Copilot settings")
			return
		}
	}
	helpers.WriteJSON(w, http.StatusOK, h.briefStatus(userID, settings))
}

func (h *Handler) handleStopActivityBriefs(w http.ResponseWriter, r *http.Request) {
	userID, ok := helpers.RequireUserID(r.Context(), w, h.authSvc)
	if !ok {
		return
	}
	err := h.useBriefSettings(
		r.Context(),
		func(settings copilotsettings.Settings, _ context.Context) error {
			h.briefBatchMu.Lock()
			defer h.briefBatchMu.Unlock()
			if h.briefQueue != nil {
				h.briefQueue.Stop(userID)
			}
			helpers.WriteJSON(w, http.StatusOK, h.briefStatus(userID, settings))
			return nil
		},
	)
	if err != nil {
		h.activityBriefError(w, err)
	}
}

func (h *Handler) handleActivityBriefBatch(w http.ResponseWriter, r *http.Request) {
	userID, ok := helpers.RequireUserID(r.Context(), w, h.authSvc)
	if !ok {
		return
	}
	var input struct {
		NotificationID string  `json:"notificationId"`
		Query          string  `json:"query"`
		RepositoryIDs  []int64 `json:"repositoryIds"`
		Days           int     `json:"days"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		helpers.WriteError(w, http.StatusBadRequest, "expected query, repositoryIds and days")
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) ||
		(input.Days != 1 && input.Days != 3 && input.Days != 7 && input.Days != 14 && input.Days != 30) {
		helpers.WriteError(
			w,
			http.StatusBadRequest,
			"days must be 1, 3, 7, 14 or 30; no trailing data allowed",
		)
		return
	}
	if input.NotificationID != "" && (input.Query != "" || len(input.RepositoryIDs) > 0) {
		helpers.WriteError(w, http.StatusBadRequest, "notificationId cannot be combined with view filters")
		return
	}
	user, err := h.authSvc.GetUser(r.Context())
	if err != nil || user == nil || user.GithubUserID != userID || user.GithubUsername == "" {
		if err != nil {
			h.logger.Warn("failed to obtain summary reader identity", zap.Error(err))
		}
		helpers.WriteError(
			w,
			http.StatusServiceUnavailable,
			"GitHub username is required to classify summary recipients.",
		)
		return
	}
	err = h.useBriefSettings(
		r.Context(),
		func(settings copilotsettings.Settings, _ context.Context) error {
			if !settings.HighlightsEnabled() || h.briefFactory == nil || h.briefQueue == nil {
				return activitybrief.ErrUnavailable
			}
			if input.NotificationID != "" {
				notification, err := h.notifications.GetByGithubID(r.Context(), userID, input.NotificationID)
				if errors.Is(err, sql.ErrNoRows) {
					helpers.WriteError(w, http.StatusNotFound, "notification not found")
					return nil
				}
				if err != nil {
					return err
				}
				kind := strings.ToLower(notification.SubjectType)
				if kind != "pullrequest" && kind != "pr" && kind != "issue" && kind != "discussion" {
					return activitybrief.ErrUnsupported
				}
				h.briefBatchMu.Lock()
				defer h.briefBatchMu.Unlock()
				selected := models.Notification{GithubID: notification.GithubID}
				added, err := h.briefQueue.Enqueue(activitybrief.Key{
					UserID: userID, NotificationID: notification.GithubID,
					Version: notification.EffectiveSortDate.UTC().Format(time.RFC3339Nano), Days: input.Days,
				}, h.runActivityBrief(userID, user.GithubUsername, selected, h.configuredBriefModel(settings)))
				if err != nil {
					helpers.WriteError(w, http.StatusTooManyRequests, err.Error())
					return nil
				}
				response := h.briefStatus(userID, settings)
				if added {
					response.Queued = 1
				}
				helpers.WriteJSON(w, http.StatusAccepted, response)
				return nil
			}
			h.enqueueActivityBriefBatch(
				w,
				r,
				userID,
				user.GithubUsername,
				input.Query,
				input.RepositoryIDs,
				input.Days,
				settings,
			)
			return nil
		},
	)
	if err != nil {
		h.activityBriefError(w, err)
	}
}

func (h *Handler) enqueueActivityBriefBatch(
	w http.ResponseWriter, r *http.Request,
	userID, reader, query string, repositoryIDs []int64, days int,
	settings copilotsettings.Settings,
) {
	h.briefBatchMu.Lock()
	defer h.briefBatchMu.Unlock()
	queued := 0
	for page := 1; queued < 10; page++ {
		list, err := h.notifications.ListNotifications(r.Context(), userID, models.ListOptions{
			Query: query, RepositoryIDs: normalizeRepositoryIDs(repositoryIDs),
			Page: page, PageSize: 100,
		})
		if err != nil {
			h.logger.Warn("failed to select activity summaries", zap.Error(err))
			if queued > 0 {
				response := h.briefStatus(userID, settings)
				response.Queued = queued
				response.Notice = "Stopped selecting further summaries: " + err.Error()
				helpers.WriteJSON(w, http.StatusAccepted, response)
				return
			}
			helpers.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		for _, notification := range list.Notifications {
			kind := strings.ToLower(notification.SubjectType)
			if kind != "pullrequest" && kind != "pr" && kind != "issue" && kind != "discussion" {
				continue
			}
			version := notification.EffectiveSortDate.UTC().Format(time.RFC3339Nano)
			added, err := h.briefQueue.Enqueue(activitybrief.Key{
				UserID: userID, NotificationID: notification.GithubID, Version: version, Days: days,
			}, h.runActivityBrief(userID, reader, notification, h.configuredBriefModel(settings)))
			if err != nil {
				if queued > 0 {
					response := h.briefStatus(userID, settings)
					response.Queued = queued
					response.Notice = err.Error()
					helpers.WriteJSON(w, http.StatusAccepted, response)
				} else {
					helpers.WriteError(w, http.StatusTooManyRequests, err.Error())
				}
				return
			}
			if added {
				queued++
				if queued == 10 {
					break
				}
			}
		}
		if len(list.Notifications) == 0 || int64(page*list.PageSize) >= list.Total {
			break
		}
	}
	response := h.briefStatus(userID, settings)
	response.Queued = queued
	helpers.WriteJSON(w, http.StatusAccepted, response)
}

func (h *Handler) runActivityBrief(
	userID, reader string,
	selected models.Notification,
	model activitybrief.Summarizer,
) func(context.Context, time.Time) (activitybrief.Result, error) {
	return func(ctx context.Context, since time.Time) (activitybrief.Result, error) {
		notification, err := h.notifications.GetByGithubID(ctx, userID, selected.GithubID)
		if err != nil {
			return activitybrief.Result{}, err
		}
		snapshot, err := activitybrief.Collect(ctx, h.githubClient, notification, since, time.Now())
		if err != nil {
			h.logger.Warn(
				"activity collection failed",
				zap.String("notification", selected.GithubID),
				zap.Error(err),
			)
			return activitybrief.Result{}, err
		}
		snapshot.ReaderUsername = reader
		snapshot.Author = notification.AuthorLogin.String
		snapshot.Warnings[0] = "User-selected maximum lookback, not a recorded archive baseline."
		result, err := activitybrief.Build(ctx, snapshot, true, model)
		if err != nil {
			h.logger.Warn(
				"activity summarization failed",
				zap.String("notification", selected.GithubID),
				zap.Error(err),
			)
		}
		return result, err
	}
}
