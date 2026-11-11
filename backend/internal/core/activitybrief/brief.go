// Package activitybrief prepares read-only, since-cutoff notification briefs.
package activitybrief

import (
	"context"
	"errors"
	"slices"
	"time"
)

var (
	ErrInvalid     = errors.New("invalid activity brief request")
	ErrUnavailable = errors.New("activity briefs are not configured")
	ErrUnsupported = errors.New("notification type does not support activity briefs")
)

// Evidence IDs are server-owned and shared by deterministic kinds and model citations.
type Evidence struct {
	ID       string     `json:"id"`
	Kind     string     `json:"kind"`
	Change   string     `json:"change"`
	At       *time.Time `json:"at,omitempty"`
	Actor    string     `json:"actor,omitempty"`
	Body     string     `json:"body,omitempty"`
	State    string     `json:"state,omitempty"`
	URL      string     `json:"url,omitempty"`
	ParentID string     `json:"parentId,omitempty"`
	Detail   string     `json:"detail,omitempty"`
}

type Snapshot struct {
	ReaderUsername string     `json:"readerUsername"`
	Author         string     `json:"author,omitempty"`
	NotificationID string     `json:"notificationId"`
	Title          string     `json:"title"`
	Since          time.Time  `json:"since"`
	FetchedAt      time.Time  `json:"fetchedAt"`
	New            []Evidence `json:"newActivity"`
	Context        []Evidence `json:"olderContext"`
	Warnings       []string   `json:"warnings"`
}

type Bullet struct {
	Text        string   `json:"text"`
	EvidenceIDs []string `json:"evidenceIds"`
	Recipient   string   `json:"recipient"`
}

type Result struct {
	Snapshot
	BaselineSource string   `json:"baselineSource"`
	EventKinds     []string `json:"eventKinds"`
	Bullets        []Bullet `json:"bullets"`
	Summarized     bool     `json:"summarized"`
}

type Summarizer interface {
	Summarize(context.Context, Snapshot) ([]Bullet, error)
}

func Build(ctx context.Context, snapshot Snapshot, summarize bool, model Summarizer) (Result, error) {
	result := Result{
		Snapshot: snapshot, BaselineSource: "explicit_cutoff",
		EventKinds: []string{}, Bullets: []Bullet{},
	}
	for _, event := range snapshot.New {
		result.EventKinds = append(result.EventKinds, event.Kind)
	}
	slices.Sort(result.EventKinds)
	result.EventKinds = slices.Compact(result.EventKinds)
	if !summarize {
		return result, nil
	}
	if model == nil {
		return result, ErrUnavailable
	}
	if len(snapshot.New) == 0 {
		result.Summarized = true
		return result, nil
	}
	bullets, err := model.Summarize(ctx, snapshot)
	if err != nil {
		return result, err
	}
	result.Bullets = bullets
	result.Summarized = true
	return result, nil
}
