package activitybrief

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func waitStatus(t *testing.T, queue *Queue, user, id, status string) Entry {
	t.Helper()
	var found Entry
	require.Eventually(t, func() bool {
		for _, entry := range queue.Entries(user) {
			if entry.NotificationID == id && entry.Status == status {
				found = entry
				return true
			}
		}
		return false
	}, time.Second, time.Millisecond)
	return found
}

func TestQueueDedupIsolationAndCancellation(t *testing.T) {
	queue := NewQueue()
	t.Cleanup(func() { queue.Stop("user"); queue.Stop("other") })
	key := Key{UserID: "user", NotificationID: "first", Days: 3, Version: "v1"}
	started := make(chan struct{})
	added, err := queue.Enqueue(key, func(ctx context.Context, since time.Time) (Result, error) {
		close(started)
		<-ctx.Done()
		return Result{}, ctx.Err()
	})
	require.NoError(t, err)
	require.True(t, added)
	<-started
	added, err = queue.Enqueue(key, func(context.Context, time.Time) (Result, error) {
		t.Error("duplicate invoked")
		return Result{}, nil
	})
	require.NoError(t, err)
	require.False(t, added)
	next := Key{UserID: "other", NotificationID: "second", Days: 3, Version: "v1"}
	_, err = queue.Enqueue(next, func(context.Context, time.Time) (Result, error) {
		return Result{Bullets: []Bullet{{Text: "@reader agreed", Recipient: "other_activity"}}}, nil
	})
	require.NoError(t, err)
	require.Len(t, queue.Entries("user"), 1)
	require.Len(t, queue.Entries("other"), 1)
	entry := waitStatus(t, queue, "other", "second", "ready")
	require.Len(t, entry.Bullets, 1)
	queue.Stop("user")
	waitStatus(t, queue, "user", "first", "cancelled")
	require.Equal(t, "ready", queue.Entries("other")[0].Status)
	require.Empty(t, queue.Entries("unknown"))
}

func TestQueueRunsTenConcurrentlyAndKeepsEleventhWaiting(t *testing.T) {
	queue := NewQueue()
	t.Cleanup(func() { queue.Stop("user") })
	started := make(chan struct{}, maxWorkers+1)
	for i := 0; i <= maxWorkers; i++ {
		_, err := queue.Enqueue(Key{UserID: "user", NotificationID: fmt.Sprint(i)}, func(ctx context.Context, _ time.Time) (Result, error) {
			started <- struct{}{}
			<-ctx.Done()
			return Result{}, ctx.Err()
		})
		require.NoError(t, err)
	}
	require.Eventually(t, func() bool { return len(started) == maxWorkers }, time.Second, time.Millisecond)
	counts := map[string]int{}
	for _, entry := range queue.Entries("user") {
		counts[entry.Status]++
	}
	require.Equal(t, maxWorkers, counts["running"])
	require.Equal(t, 1, counts["queued"])
	queue.Stop("user")
	for _, entry := range queue.Entries("user") {
		require.Equal(t, "cancelled", entry.Status)
	}
	require.Len(t, started, maxWorkers, "stopped waiting job must not start")
}
func TestQueueFailureRetriesOnlyOnExplicitEnqueue(t *testing.T) {
	queue := NewQueue()
	key := Key{UserID: "user", NotificationID: "first", Days: 3, Version: "v1"}
	calls := 0
	run := func(context.Context, time.Time) (Result, error) {
		calls++
		return Result{}, errors.New("provider unavailable")
	}
	_, err := queue.Enqueue(key, run)
	require.NoError(t, err)
	entry := waitStatus(t, queue, "user", "first", "failed")
	require.Contains(t, entry.Error, "provider unavailable")
	for i := 0; i < 10; i++ {
		require.Len(t, queue.Entries("user"), 1)
	}
	require.Equal(t, 1, calls)
	added, err := queue.Enqueue(key, run)
	require.NoError(t, err)
	require.True(t, added)
	waitStatus(t, queue, "user", "first", "failed")
	require.Equal(t, 2, calls)
}

func TestQueueHorizonVersionAndEmptyResultCaching(t *testing.T) {
	queue := NewQueue()
	now := time.Now().UTC()
	queue.now = func() time.Time { return now }
	run := func(_ context.Context, since time.Time) (Result, error) {
		require.Equal(t, now.Add(-3*24*time.Hour), since)
		return Result{Bullets: []Bullet{}}, nil
	}
	key := Key{UserID: "user", NotificationID: "first", Days: 3, Version: "v1"}
	_, err := queue.Enqueue(key, run)
	require.NoError(t, err)
	waitStatus(t, queue, "user", "first", "ready")
	added, err := queue.Enqueue(key, run)
	require.NoError(t, err)
	require.False(t, added, "empty successful output is still cached")
	key.Version = "v2"
	added, err = queue.Enqueue(key, run)
	require.NoError(t, err)
	require.True(t, added)
	require.Eventually(t, func() bool {
		entries := queue.Entries("user")
		return len(entries) == 2 && entries[0].Status == "ready" && entries[1].Status == "ready"
	}, time.Second, time.Millisecond)
	queue.mu.Lock()
	now = now.Add(cacheLifetime)
	queue.mu.Unlock()
	require.Empty(t, queue.Entries("user"))
	key.Days = 7
	added, err = queue.Enqueue(key, func(context.Context, time.Time) (Result, error) { return Result{}, nil })
	require.NoError(t, err)
	require.True(t, added)
	waitStatus(t, queue, "user", "first", "ready")
}

func TestQueueBoundsAndStopRemovesPendingWork(t *testing.T) {
	queue := NewQueue()
	t.Cleanup(func() { queue.Stop("user") })
	started := make(chan struct{})
	_, err := queue.Enqueue(Key{UserID: "user", NotificationID: "running"}, func(ctx context.Context, _ time.Time) (Result, error) {
		close(started)
		<-ctx.Done()
		return Result{}, ctx.Err()
	})
	require.NoError(t, err)
	<-started
	for i := 1; i < maxPending; i++ {
		_, err := queue.Enqueue(Key{UserID: "user", NotificationID: fmt.Sprint(i)}, func(ctx context.Context, _ time.Time) (Result, error) {
			<-ctx.Done()
			return Result{}, ctx.Err()
		})
		require.NoError(t, err)
	}

	_, err = queue.Enqueue(Key{UserID: "user", NotificationID: "overflow"}, nil)
	require.ErrorIs(t, err, ErrQueueFull)
	queue.Stop("user")
	for _, entry := range queue.Entries("user") {
		require.Equal(t, "cancelled", entry.Status)
	}
	queue.mu.Lock()
	require.Empty(t, queue.pending)
	queue.mu.Unlock()
}

func TestQueueCloseJoinsOwnedWorkAndRejectsNewJobs(t *testing.T) {
	queue := NewQueue()
	started, finished := make(chan struct{}), make(chan struct{})
	_, err := queue.Enqueue(Key{UserID: "user", NotificationID: "running"}, func(ctx context.Context, _ time.Time) (Result, error) {
		close(started)
		<-ctx.Done()
		close(finished)
		return Result{}, ctx.Err()
	})
	require.NoError(t, err)
	<-started
	queue.Close()
	select {
	case <-finished:
	default:
		t.Fatal("shutdown returned before owned work stopped")
	}
	_, err = queue.Enqueue(Key{UserID: "other", NotificationID: "new"}, nil)
	require.ErrorContains(t, err, "stopping")
	require.Zero(t, queue.workers)
}
