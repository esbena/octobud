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
	Enabled       bool                  `json:"enabled"`
	AutoSummarize bool                  `json:"autoSummarize"`
	Entries       []activitybrief.Entry `json:"entries"`
	Queued        int                   `json:"queued"`
	Skipped       int                   `json:"skipped"`
	Notice        string                `json:"notice,omitempty"`
}

func (h *Handler) briefStatus(
	userID string, settings copilotsettings.Settings,
) briefStatusResponse {
	entries := []activitybrief.Entry{}
	if h.briefQueue != nil {
		entries = h.briefQueue.Entries(userID)
	}
	return briefStatusResponse{
		Enabled: settings.HighlightsEnabled(),
		AutoSummarize: settings.HighlightsEnabled() &&
			settings.ActivityHighlights.AutoSummarizeVisible,
		Entries: entries,
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
		NotificationID string    `json:"notificationId"`
		Query          string    `json:"query"`
		RepositoryIDs  []int64   `json:"repositoryIds"`
		Selection      string    `json:"selection"`
		TimeZone       string    `json:"timeZone"`
		Since          time.Time `json:"since"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		helpers.WriteError(w, http.StatusBadRequest, "expected a cutoff selection and timeZone")
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		helpers.WriteError(w, http.StatusBadRequest, "unexpected trailing request data")
		return
	}
	location, err := time.LoadLocation(input.TimeZone)
	if err != nil {
		helpers.WriteError(w, http.StatusBadRequest, "timeZone must be a valid IANA time zone")
		return
	}
	if !activitybrief.ValidSelection(input.Selection) {
		helpers.WriteError(w, http.StatusBadRequest, "selection is not a supported catch-up range")
		return
	}
	if !input.Since.IsZero() && input.Since.After(time.Now()) {
		helpers.WriteError(w, http.StatusBadRequest, "since cannot be in the future")
		return
	}
	if input.NotificationID != "" && (input.Query != "" || len(input.RepositoryIDs) > 0) {
		helpers.WriteError(w, http.StatusBadRequest, "notificationId cannot be combined with view filters")
		return
	}
	if input.NotificationID == "" && !input.Since.IsZero() {
		helpers.WriteError(w, http.StatusBadRequest, "since can only be used with notificationId")
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
				var lastReadAt, lastArchivedAt *time.Time
				if notification.TimelineLastSeenAt.Valid {
					lastReadAt = &notification.TimelineLastSeenAt.Time
				}
				if notification.LastArchivedAt.Valid {
					lastArchivedAt = &notification.LastArchivedAt.Time
				}
				since := input.Since.UTC()
				if input.Since.IsZero() {
					since, err = activitybrief.ResolveCutoff(
						input.Selection, lastReadAt, lastArchivedAt, time.Now(), location,
					)
					if err != nil {
						return err
					}
				}
				h.briefBatchMu.Lock()
				defer h.briefBatchMu.Unlock()
				selected := models.Notification{GithubID: notification.GithubID}
				added, err := h.briefQueue.Enqueue(activitybrief.Key{
					UserID: userID, NotificationID: notification.GithubID,
					Version:   notification.EffectiveSortDate.UTC().Format(time.RFC3339Nano),
					Selection: input.Selection, Since: since,
				}, h.runActivityBrief(userID, user.GithubUsername, selected, input.Selection,
					h.briefFactory(settings.ActivityHighlights.Model)))
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
				input.Selection,
				location,
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
	userID, reader, query string, repositoryIDs []int64, selection string, location *time.Location,
	settings copilotsettings.Settings,
) {
	h.briefBatchMu.Lock()
	defer h.briefBatchMu.Unlock()
	queued := 0
	skipped := 0
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
			since, err := activitybrief.ResolveCutoff(
				selection, notification.TimelineLastSeenAt, notification.LastArchivedAt,
				time.Now(), location,
			)
			if errors.Is(err, activitybrief.ErrNoBaseline) {
				skipped++
				continue
			}
			if err != nil {
				h.logger.Warn("failed to resolve activity summary cutoff", zap.Error(err))
				continue
			}
			version := notification.EffectiveSortDate.UTC().Format(time.RFC3339Nano)
			added, err := h.briefQueue.Enqueue(activitybrief.Key{
				UserID: userID, NotificationID: notification.GithubID, Version: version,
				Selection: selection, Since: since,
			}, h.runActivityBrief(userID, reader, notification, selection,
				h.briefFactory(settings.ActivityHighlights.Model)))
			if err != nil {
				if queued > 0 {
					response := h.briefStatus(userID, settings)
					response.Queued = queued
					response.Skipped = skipped
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
	response.Skipped = skipped
	helpers.WriteJSON(w, http.StatusAccepted, response)
}

func (h *Handler) runActivityBrief(
	userID, reader string,
	selected models.Notification,
	selection string,
	model activitybrief.Summarizer,
) func(context.Context, time.Time) (activitybrief.Result, error) {
	return func(ctx context.Context, since time.Time) (activitybrief.Result, error) {
		notification, err := h.notifications.GetByGithubID(ctx, userID, selected.GithubID)
		if err != nil {
			return activitybrief.Result{}, err
		}
		snapshot, err := activitybrief.CollectForReader(
			ctx, h.githubClient, notification, since, time.Now(),
		)
		if err != nil {
			h.logger.Warn(
				"activity collection failed",
				zap.String("notification", selected.GithubID),
				zap.Error(err),
			)
			return activitybrief.Result{}, err
		}
		snapshot.ReaderUsername = reader
		if notification.AuthorLogin.Valid {
			snapshot.Author = notification.AuthorLogin.String
		}
		if selection == "my_last_comment" {
			snapshot, err = activitybrief.RebaseSinceLastComment(snapshot, reader)
			if err != nil {
				return activitybrief.Result{}, err
			}
		}
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
