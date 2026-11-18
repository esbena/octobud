package notifications

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/octobud-hq/octobud/backend/internal/core/activitybrief"
	"github.com/octobud-hq/octobud/backend/internal/db"
	"github.com/octobud-hq/octobud/backend/internal/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func briefQueueTestKey(user, id string) activitybrief.Key {
	return activitybrief.Key{
		UserID: user, NotificationID: id, Selection: "past_7_days",
		Since: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC),
	}
}

func TestIndividualActivityBriefUsesSharedQueueAndCache(t *testing.T) {
	handler, notifications, _, auth := setupTestHandler(gomock.NewController(t))
	handler.withActivityBriefs(briefModel(nil))
	configureBriefTestSettings(t, handler, true)
	auth.EXPECT().GetUser(gomock.Any()).Return(&models.User{GithubUserID: "owner", GithubUsername: "reader"}, nil).AnyTimes()
	at := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	notifications.EXPECT().GetByGithubID(gomock.Any(), "owner", "selected").Return(db.Notification{
		GithubID: "selected", SubjectType: "Issue", EffectiveSortDate: at,
	}, nil).Times(2)
	_, err := handler.briefQueue.Enqueue(activitybrief.Key{
		UserID: "owner", NotificationID: "selected", Version: at.Format(time.RFC3339Nano),
		Selection: "past_7_days", Since: at.Add(-7 * 24 * time.Hour),
	}, func(ctx context.Context, _ time.Time) (activitybrief.Result, error) {
		<-ctx.Done()
		return activitybrief.Result{}, ctx.Err()
	})
	require.NoError(t, err)
	for range 2 {
		response := httptest.NewRecorder()
		handler.handleActivityBriefBatch(response, briefRequest(`{"notificationId":"selected","selection":"past_7_days","timeZone":"UTC"}`))
		require.Equal(t, http.StatusAccepted, response.Code, response.Body.String())
		var status briefStatusResponse
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &status))
		require.Zero(t, status.Queued)
		require.Len(t, status.Entries, 1)
	}
}

func TestIndividualActivityBriefQueuesOnlySelectedNotification(t *testing.T) {
	handler, notifications, _, auth := setupTestHandler(gomock.NewController(t))
	handler.withActivityBriefs(briefModel(nil))
	configureBriefTestSettings(t, handler, true)
	auth.EXPECT().GetUser(gomock.Any()).Return(&models.User{GithubUserID: "owner", GithubUsername: "reader"}, nil).AnyTimes()
	started := make(chan struct{}, 10)
	for i := range 10 {
		_, err := handler.briefQueue.Enqueue(briefQueueTestKey("blocker", fmt.Sprint(i)),
			func(ctx context.Context, _ time.Time) (activitybrief.Result, error) {
				started <- struct{}{}
				<-ctx.Done()
				return activitybrief.Result{}, ctx.Err()
			})
		require.NoError(t, err)
	}
	require.Eventually(t, func() bool { return len(started) == 10 }, time.Second, time.Millisecond)
	at := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	notifications.EXPECT().GetByGithubID(gomock.Any(), "owner", "selected").Return(db.Notification{
		GithubID: "selected", SubjectType: "Issue", EffectiveSortDate: at,
	}, nil)
	response := httptest.NewRecorder()
	handler.handleActivityBriefBatch(response, briefRequest(`{"notificationId":"selected","selection":"past_7_days","timeZone":"UTC"}`))
	require.Equal(t, http.StatusAccepted, response.Code, response.Body.String())
	var status briefStatusResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &status))
	require.Equal(t, 1, status.Queued)
	require.Len(t, status.Entries, 1)
	require.Equal(t, "selected", status.Entries[0].NotificationID)
	require.Equal(t, "past_7_days", status.Entries[0].Selection)
	require.Equal(t, at.Format(time.RFC3339Nano), status.Entries[0].Version)
}

func TestIndividualActivityBriefUsesRequestedCutoff(t *testing.T) {
	handler, notifications, _, auth := setupTestHandler(gomock.NewController(t))
	handler.withActivityBriefs(briefModel(nil))
	configureBriefTestSettings(t, handler, true)
	auth.EXPECT().GetUser(gomock.Any()).Return(
		&models.User{GithubUserID: "owner", GithubUsername: "reader"}, nil,
	).AnyTimes()
	now := time.Now().UTC().Truncate(time.Second)
	since := now.Add(-24 * time.Hour)
	notifications.EXPECT().GetByGithubID(gomock.Any(), "owner", "selected").Return(db.Notification{
		GithubID: "selected", SubjectType: "Issue", EffectiveSortDate: now,
	}, nil).Times(1)
	key := activitybrief.Key{
		UserID: "owner", NotificationID: "selected", Version: now.Format(time.RFC3339Nano),
		Selection: "last_read", Since: since,
	}
	started := make(chan struct{})
	_, err := handler.briefQueue.Enqueue(key, func(ctx context.Context, _ time.Time) (activitybrief.Result, error) {
		close(started)
		<-ctx.Done()
		return activitybrief.Result{}, ctx.Err()
	})
	require.NoError(t, err)
	<-started
	t.Cleanup(func() { handler.briefQueue.Stop("owner") })
	response := httptest.NewRecorder()
	body, err := json.Marshal(map[string]any{
		"notificationId": "selected",
		"selection":      "last_read",
		"timeZone":       "UTC",
		"since":          since,
	})
	require.NoError(t, err)
	handler.handleActivityBriefBatch(response, briefRequest(string(body)))
	require.Equal(t, http.StatusAccepted, response.Code, response.Body.String())
	var status briefStatusResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &status))
	require.Len(t, status.Entries, 1)
	require.Equal(t, since, status.Entries[0].Since)
	require.Zero(t, status.Queued)
}

func TestIndividualActivityBriefRejectsMissingNotificationAndMixedScope(t *testing.T) {
	handler, notifications, _, auth := setupTestHandler(gomock.NewController(t))
	handler.withActivityBriefs(briefModel(nil))
	configureBriefTestSettings(t, handler, true)
	auth.EXPECT().GetUser(gomock.Any()).Return(&models.User{GithubUserID: "owner", GithubUsername: "reader"}, nil).AnyTimes()
	notifications.EXPECT().GetByGithubID(gomock.Any(), "owner", "missing").Return(db.Notification{}, sql.ErrNoRows)
	for _, test := range []struct {
		body   string
		status int
	}{
		{`{"notificationId":"missing","selection":"past_7_days","timeZone":"UTC"}`, http.StatusNotFound},
		{`{"notificationId":"selected","query":"in:inbox","selection":"past_7_days","timeZone":"UTC"}`, http.StatusBadRequest},
	} {
		response := httptest.NewRecorder()
		handler.handleActivityBriefBatch(response, briefRequest(test.body))
		require.Equal(t, test.status, response.Code, response.Body.String())
	}
	require.Empty(t, handler.briefQueue.Entries("owner"))
}

func TestBatchFillsGapsAcrossPagesAndRepeatedClicksExtendQueue(t *testing.T) {
	handler, notifications, _, auth := setupTestHandler(gomock.NewController(t))
	handler.withActivityBriefs(briefModel(func(context.Context, activitybrief.Snapshot) ([]activitybrief.Bullet, error) {
		t.Error("batch selection must not synchronously invoke model")
		return nil, nil
	}))
	auth.EXPECT().GetUser(gomock.Any()).Return(&models.User{GithubUserID: "owner", GithubUsername: "reader"}, nil).AnyTimes()
	configureBriefTestSettings(t, handler, true)
	queue := handler.briefQueue
	t.Cleanup(func() { queue.Stop("owner"); queue.Stop("blocker") })
	at := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	version := at.Format(time.RFC3339Nano)
	items := make([]models.Notification, 125)
	for i := range items {
		items[i] = models.Notification{GithubID: fmt.Sprint(i), SubjectType: "PullRequest", EffectiveSortDate: at}
		if i < 100 && i != 7 {
			key := briefQueueTestKey("owner", fmt.Sprint(i))
			key.Version = version
			_, err := queue.Enqueue(key,
				func(context.Context, time.Time) (activitybrief.Result, error) { return activitybrief.Result{}, nil })
			require.NoError(t, err)
		}
	}
	require.Eventually(t, func() bool {
		for _, entry := range queue.Entries("owner") {
			if entry.Status != "ready" {
				return false
			}
		}
		return true
	}, time.Second, time.Millisecond)
	started := make(chan struct{}, 10)
	for i := 0; i < 10; i++ {
		_, err := queue.Enqueue(briefQueueTestKey("blocker", fmt.Sprint(i)), func(ctx context.Context, _ time.Time) (activitybrief.Result, error) {
			started <- struct{}{}
			<-ctx.Done()
			return activitybrief.Result{}, ctx.Err()
		})
		require.NoError(t, err)
	}
	require.Eventually(t, func() bool { return len(started) == 10 }, time.Second, time.Millisecond)
	for page := 1; page <= 2; page++ {
		first, last := (page-1)*100, min(page*100, len(items))
		notifications.EXPECT().ListNotifications(gomock.Any(), "owner", models.ListOptions{
			Query: "is:unarchived", RepositoryIDs: []int64{42}, Page: page, PageSize: 100,
		}).Return(models.ListDetailsResult{Notifications: items[first:last], Total: 125, Page: page, PageSize: 100}, nil).Times(2)
	}
	for click := 0; click < 2; click++ {
		response := httptest.NewRecorder()
		handler.handleActivityBriefBatch(response, briefRequest(`{"query":"is:unarchived","repositoryIds":[42],"selection":"past_7_days","timeZone":"UTC"}`))
		require.Equal(t, http.StatusAccepted, response.Code, response.Body.String())
		var status briefStatusResponse
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &status))
		require.Equal(t, 10, status.Queued)
		pending := map[string]bool{}
		for _, entry := range status.Entries {
			if entry.Status == "queued" {
				pending[entry.NotificationID] = true
			}
		}
		require.Len(t, pending, (click+1)*10)
		require.True(t, pending["7"], "fill eighth-position gap first")
		require.True(t, pending[fmt.Sprint(108+click*10)])
		require.False(t, pending[fmt.Sprint(109+click*10)])
	}
	handler.handleStopActivityBriefs(httptest.NewRecorder(), briefRequest(""))
	for _, entry := range queue.Entries("owner") {
		require.NotEqual(t, "queued", entry.Status)
	}
}

func TestBatchStatusReadDoesNotFetchOrSummarize(t *testing.T) {
	handler, _, _, auth := setupTestHandler(gomock.NewController(t))
	auth.EXPECT().GetUser(gomock.Any()).Return(&models.User{GithubUserID: "owner"}, nil)
	response := httptest.NewRecorder()
	handler.handleActivityBriefStatus(response, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, response.Code)
	var status briefStatusResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &status))
	require.False(t, status.Enabled)
	require.Empty(t, status.Entries)
}

func TestBatchQueuesAllHistoryForNotificationsWithoutAReadCheckpoint(t *testing.T) {
	handler, notifications, _, auth := setupTestHandler(gomock.NewController(t))
	handler.withActivityBriefs(briefModel(nil))
	t.Cleanup(handler.briefQueue.Close)
	started := make(chan struct{}, 10)
	for i := range 10 {
		_, err := handler.briefQueue.Enqueue(briefQueueTestKey("blocker", fmt.Sprint(i)),
			func(ctx context.Context, _ time.Time) (activitybrief.Result, error) {
				started <- struct{}{}
				<-ctx.Done()
				return activitybrief.Result{}, ctx.Err()
			})
		require.NoError(t, err)
	}
	require.Eventually(t, func() bool { return len(started) == 10 }, time.Second, time.Millisecond)
	configureBriefTestSettings(t, handler, true)
	auth.EXPECT().GetUser(gomock.Any()).Return(
		&models.User{GithubUserID: "owner", GithubUsername: "reader"}, nil,
	).AnyTimes()
	notifications.EXPECT().ListNotifications(gomock.Any(), "owner", gomock.Any()).Return(
		models.ListDetailsResult{
			Notifications: []models.Notification{{GithubID: "one", SubjectType: "Issue"}},
			Total:         1, Page: 1, PageSize: 100,
		},
		nil,
	)
	response := httptest.NewRecorder()
	handler.handleActivityBriefBatch(
		response,
		briefRequest(`{"query":"in:inbox","selection":"last_read","timeZone":"UTC"}`),
	)
	require.Equal(t, http.StatusAccepted, response.Code)
	var status briefStatusResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &status))
	require.Equal(t, 1, status.Queued)
	require.Zero(t, status.Skipped)
	require.Len(t, status.Entries, 1)
	require.Equal(t, time.Unix(0, 0).UTC(), status.Entries[0].Since)
}

func TestBatchReportsAlreadyQueuedJobsWhenLaterPageFails(t *testing.T) {
	handler, notifications, _, auth := setupTestHandler(gomock.NewController(t))
	handler.withActivityBriefs(briefModel(nil))
	configureBriefTestSettings(t, handler, true)
	t.Cleanup(handler.briefQueue.Close)
	auth.EXPECT().GetUser(gomock.Any()).Return(&models.User{GithubUserID: "owner", GithubUsername: "reader"}, nil).AnyTimes()
	started := make(chan struct{}, 10)
	for i := 0; i < 10; i++ {
		_, err := handler.briefQueue.Enqueue(briefQueueTestKey("blocker", fmt.Sprint(i)),
			func(ctx context.Context, _ time.Time) (activitybrief.Result, error) {
				started <- struct{}{}
				<-ctx.Done()
				return activitybrief.Result{}, ctx.Err()
			})
		require.NoError(t, err)
	}
	require.Eventually(t, func() bool { return len(started) == 10 }, time.Second, time.Millisecond)
	notifications.EXPECT().ListNotifications(gomock.Any(), "owner", gomock.Any()).Return(models.ListDetailsResult{
		Notifications: []models.Notification{{GithubID: "selected", SubjectType: "Issue"}},
		Total:         101, Page: 1, PageSize: 100,
	}, nil)
	notifications.EXPECT().ListNotifications(gomock.Any(), "owner", gomock.Any()).Return(models.ListDetailsResult{}, errors.New("database interrupted"))
	response := httptest.NewRecorder()
	handler.handleActivityBriefBatch(response, briefRequest(`{"query":"in:inbox","selection":"past_7_days","timeZone":"UTC"}`))
	require.Equal(t, http.StatusAccepted, response.Code)
	var status briefStatusResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &status))
	require.Equal(t, 1, status.Queued)
	require.Contains(t, status.Notice, "database interrupted")
	require.Len(t, status.Entries, 1)
	require.Equal(t, "queued", status.Entries[0].Status)
}

func TestBatchRejectsInvalidHorizonsAndTrailingDataWithoutWork(t *testing.T) {
	for _, body := range []string{
		`{"selection":"invalid","timeZone":"UTC"}`,
		`{"selection":"past_7_days","timeZone":"Invalid/Zone"}`,
		`{"selection":"past_7_days","timeZone":"UTC","limit":1000}`,
		`{"selection":"past_7_days","timeZone":"UTC"} {}`,
	} {
		t.Run(body, func(t *testing.T) {
			handler, _, _, auth := setupTestHandler(gomock.NewController(t))
			handler.withActivityBriefs(briefModel(func(context.Context, activitybrief.Snapshot) ([]activitybrief.Bullet, error) {
				t.Error("invalid request invoked model")
				return nil, nil
			}))
			auth.EXPECT().GetUser(gomock.Any()).Return(&models.User{GithubUserID: "owner"}, nil)
			response := httptest.NewRecorder()
			handler.handleActivityBriefBatch(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
			require.Equal(t, http.StatusBadRequest, response.Code)
			require.Empty(t, handler.briefQueue.Entries("owner"))
		})
	}
}
