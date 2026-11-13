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
	ErrIncomplete  = errors.New("activity evidence is incomplete")
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
	// Reader metadata keeps original chronology separate from At (the change time).
	CreatedAt *time.Time `json:"createdAt,omitempty"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
	ActorType string     `json:"actorType,omitempty"`
	RootID    string     `json:"rootId,omitempty"`
	ReviewID  string     `json:"reviewId,omitempty"`
	Path      string     `json:"path,omitempty"`
	DiffHunk  string     `json:"diffHunk,omitempty"`
	Outdated  *bool      `json:"outdated,omitempty"`
	Message   string     `json:"message,omitempty"`
}

type Snapshot struct {
	ReaderUsername  string     `json:"readerUsername"`
	Author          string     `json:"author,omitempty"`
	NotificationID  string     `json:"notificationId"`
	Title           string     `json:"title"`
	Selection       string     `json:"selection,omitempty"`
	Since           time.Time  `json:"since"`
	BaselineMessage string     `json:"baselineMessage,omitempty"`
	FetchedAt       time.Time  `json:"fetchedAt"`
	TotalEntries    int        `json:"totalEntries"`
	New             []Evidence `json:"newActivity"`
	Context         []Evidence `json:"olderContext"`
	Warnings        []string   `json:"warnings"`
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
	baselineSource := snapshot.Selection
	if baselineSource == "" {
		baselineSource = "explicit_cutoff"
	}
	result := Result{
		Snapshot: snapshot, BaselineSource: baselineSource,
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
