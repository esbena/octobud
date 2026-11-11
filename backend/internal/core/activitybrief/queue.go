package activitybrief

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	maxWorkers    = 10
	maxPending    = 100
	maxCached     = 500
	cacheLifetime = time.Hour
)

var ErrQueueFull = errors.New("summary queue is full; wait for queued summaries or stop them")

type Key struct {
	UserID         string
	NotificationID string
	Version        string
	Days           int
}

type Entry struct {
	NotificationID string    `json:"notificationId"`
	Version        string    `json:"version"`
	Days           int       `json:"days"`
	Status         string    `json:"status"`
	Since          time.Time `json:"since"`
	UpdatedAt      time.Time `json:"updatedAt"`
	Bullets        []Bullet  `json:"bullets"`
	Warnings       []string  `json:"warnings,omitempty"`
	Error          string    `json:"error,omitempty"`
}

type queuedBrief struct {
	key    Key
	entry  Entry
	run    func(context.Context, time.Time) (Result, error)
	cancel context.CancelFunc
}

// Queue owns a bounded worker pool and user-scoped process-memory results.
// Only Enqueue starts work; reading entries never invokes GitHub or inference.
type Queue struct {
	mu      sync.Mutex
	wg      sync.WaitGroup
	entries map[Key]*queuedBrief
	pending []*queuedBrief
	workers int
	closed  bool
	now     func() time.Time
}

func NewQueue() *Queue {
	return &Queue{entries: map[Key]*queuedBrief{}, now: time.Now}
}

func (q *Queue) prune() {
	now := q.now()
	for key, job := range q.entries {
		if job.entry.Status != "queued" && job.entry.Status != "running" &&
			now.Sub(job.entry.UpdatedAt) >= cacheLifetime {
			delete(q.entries, key)
		}
	}
}

func (q *Queue) Enqueue(key Key, run func(context.Context, time.Time) (Result, error)) (bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return false, errors.New("summary queue is stopping")
	}
	q.prune()
	if old := q.entries[key]; old != nil && old.entry.Status != "failed" && old.entry.Status != "cancelled" {
		return false, nil
	}
	pending := 0
	for _, job := range q.entries {
		if job.entry.Status == "queued" || job.entry.Status == "running" {
			pending++
		}
	}
	if pending >= maxPending {
		return false, ErrQueueFull
	}
	for len(q.entries) >= maxCached {
		var oldest *queuedBrief
		for _, job := range q.entries {
			if job.entry.Status != "queued" && job.entry.Status != "running" &&
				(oldest == nil || job.entry.UpdatedAt.Before(oldest.entry.UpdatedAt)) {
				oldest = job
			}
		}
		if oldest == nil {
			return false, ErrQueueFull
		}
		delete(q.entries, oldest.key)
	}
	now := q.now().UTC()
	job := &queuedBrief{key: key, run: run, entry: Entry{
		NotificationID: key.NotificationID, Version: key.Version, Days: key.Days,
		Status: "queued", Since: now.Add(-time.Duration(key.Days) * 24 * time.Hour),
		UpdatedAt: now, Bullets: []Bullet{},
	}}
	q.entries[key] = job
	q.pending = append(q.pending, job)
	if q.workers < maxWorkers {
		q.workers++
		q.wg.Add(1)
		go q.work()
	}
	return true, nil
}

func (q *Queue) work() {
	defer q.wg.Done()
	for {
		q.mu.Lock()
		if len(q.pending) == 0 {
			q.workers--
			q.mu.Unlock()
			return
		}
		job := q.pending[0]
		q.pending = q.pending[1:]
		if job.entry.Status != "queued" {
			q.mu.Unlock()
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		job.cancel = cancel
		job.entry.Status = "running"
		q.mu.Unlock()

		result, err := job.run(ctx, job.entry.Since)
		cancel()
		q.mu.Lock()
		job.cancel = nil
		if job.entry.Status != "cancelled" {
			job.entry.UpdatedAt = q.now().UTC()
			if err != nil {
				job.entry.Status = "failed"
				job.entry.Error = fmt.Sprintf("Activity summary failed: %v", err)
			} else {
				job.entry.Status = "ready"
				job.entry.Bullets = result.Bullets
				job.entry.Warnings = result.Warnings
			}
		}
		q.mu.Unlock()
	}
}

func (q *Queue) Entries(userID string) []Entry {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.prune()
	entries := []Entry{}
	for key, job := range q.entries {
		if key.UserID == userID {
			entry := job.entry
			entry.Bullets = append([]Bullet{}, entry.Bullets...)
			entry.Warnings = append([]string{}, entry.Warnings...)
			entries = append(entries, entry)
		}
	}
	return entries
}

func (q *Queue) Stop(userID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.stopLocked(userID, false)
}

func (q *Queue) Close() {
	q.mu.Lock()
	q.closed = true
	q.stopLocked("", true)
	q.mu.Unlock()
	q.wg.Wait()
}

func (q *Queue) StopAll() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.stopLocked("", true)
}

func (q *Queue) stopLocked(userID string, all bool) {
	for key, job := range q.entries {
		if (all || key.UserID == userID) && (job.entry.Status == "queued" || job.entry.Status == "running") {
			job.entry.Status = "cancelled"
			job.entry.UpdatedAt = q.now().UTC()
			if job.cancel != nil {
				job.cancel()
			}
		}
	}
	pending := q.pending[:0]
	for _, job := range q.pending {
		if job.entry.Status == "queued" {
			pending = append(pending, job)
		}
	}
	q.pending = pending
}
