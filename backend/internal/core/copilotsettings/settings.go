// Package copilotsettings owns persisted feature preferences, not credentials.
package copilotsettings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/octobud-hq/octobud/backend/internal/db"
)

type Settings struct {
	Enabled            bool               `json:"enabled"`
	ActivityHighlights ActivityHighlights `json:"activityHighlights"`
}

type ActivityHighlights struct {
	Enabled     bool   `json:"enabled"`
	DefaultDays int    `json:"defaultDays"`
	Model       string `json:"model"`
}

func Defaults() Settings {
	return Settings{ActivityHighlights: ActivityHighlights{DefaultDays: 3, Model: "gpt-6-luna"}}
}

func (s Settings) Validate() error {
	switch s.ActivityHighlights.DefaultDays {
	case 1, 3, 7, 14, 30:
	default:
		return errors.New("defaultDays must be 1, 3, 7, 14 or 30")
	}
	model := s.ActivityHighlights.Model
	if strings.TrimSpace(model) == "" || model != strings.TrimSpace(model) || len(model) > 128 {
		return errors.New(
			"model must be a nonempty model ID of at most 128 bytes without surrounding whitespace",
		)
	}
	return nil
}

func (s Settings) HighlightsEnabled() bool {
	return s.Enabled && s.ActivityHighlights.Enabled
}

type Service struct {
	store     db.Store
	mu        sync.Mutex
	work      context.Context
	cancel    context.CancelFunc
	onDisable func()
}

func New(store db.Store, onDisable func()) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{store: store, work: ctx, cancel: cancel, onDisable: onDisable}
}

func (s *Service) load(ctx context.Context) (Settings, error) {
	user, err := s.store.GetUser(ctx)
	if err != nil {
		return Settings{}, err
	}
	settings := Defaults()
	if user.CopilotSettings.Valid {
		if err := json.Unmarshal(user.CopilotSettings.RawMessage, &settings); err != nil {
			return Settings{}, fmt.Errorf("decode Copilot settings: %w", err)
		}
	}
	if err := settings.Validate(); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

func (s *Service) Get(ctx context.Context) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load(ctx)
}

// Use serializes work admission with disabling and supplies a cancellation
// context for admitted work. The callback must not wait for inference.
func (s *Service) Use(ctx context.Context, admit func(Settings, context.Context) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings, err := s.load(ctx)
	if err != nil {
		return err
	}
	return admit(settings, s.work)
}

func (s *Service) Update(ctx context.Context, settings Settings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	if _, err := s.store.UpdateUserCopilotSettings(ctx, db.NullRawMessage{RawMessage: data, Valid: true}); err != nil {
		return err
	}
	if !settings.HighlightsEnabled() {
		s.cancel()
		if s.onDisable != nil {
			s.onDisable()
		}
		s.work, s.cancel = context.WithCancel(context.Background())
	}
	return nil
}

func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancel()
}
