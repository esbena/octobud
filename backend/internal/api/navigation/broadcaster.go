// Copyright (C) 2025 Austin Beattie
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

// Package navigation implements a broadcaster for navigation events.
package navigation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
)

// NavigationEvent represents a navigation command to be sent to clients.
type NavigationEvent struct {
	URL       string    `json:"url"`
	Timestamp time.Time `json:"timestamp"`
}

// Broadcaster manages SSE connections and broadcasts navigation events.
type Broadcaster struct {
	logger   *zap.Logger
	mu       sync.RWMutex
	clients  map[chan NavigationEvent]bool
	newTab   bool
	path     string
	disabled chan struct{}
}

// NewBroadcaster creates a new navigation event broadcaster.
func NewBroadcaster(logger *zap.Logger) *Broadcaster {
	return &Broadcaster{
		logger:   logger,
		clients:  make(map[chan NavigationEvent]bool),
		disabled: make(chan struct{}),
	}
}

// NewPersistentBroadcaster stores the menu navigation preference in the data directory.
func NewPersistentBroadcaster(logger *zap.Logger, dataDir string) (*Broadcaster, error) {
	b := NewBroadcaster(logger)
	b.path = filepath.Join(dataDir, "navigation-settings.json")
	data, err := os.ReadFile(b.path)
	if errors.Is(err, os.ErrNotExist) {
		return b, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read navigation settings: %w", err)
	}
	var settings struct {
		NewTab *bool `json:"newTab"`
	}
	if err := json.Unmarshal(data, &settings); err != nil || settings.NewTab == nil {
		return nil, fmt.Errorf("invalid navigation settings: %s", b.path)
	}
	b.newTab = *settings.NewTab
	if b.newTab {
		close(b.disabled)
	}
	return b, nil
}

func (b *Broadcaster) NewTabMode() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.newTab
}

func (b *Broadcaster) SetNewTabMode(newTab bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.newTab == newTab {
		return nil
	}
	if b.path != "" {
		data, err := json.Marshal(struct {
			NewTab bool `json:"newTab"`
		}{newTab})
		if err != nil {
			return err
		}
		f, err := os.CreateTemp(filepath.Dir(b.path), ".navigation-settings-*")
		if err != nil {
			return err
		}
		defer os.Remove(f.Name())
		if _, err = f.Write(data); err != nil {
			f.Close()
			return err
		}
		if err = f.Close(); err != nil {
			return err
		}
		if err = os.Rename(f.Name(), b.path); err != nil {
			return err
		}
	}
	b.newTab = newTab
	if newTab {
		close(b.disabled)
	} else {
		b.disabled = make(chan struct{})
	}
	return nil
}

// SubscribeIfEnabled observes the setting and subscribes under the same lock.
func (b *Broadcaster) SubscribeIfEnabled() (chan NavigationEvent, <-chan struct{}, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.newTab {
		return nil, nil, false
	}
	ch := make(chan NavigationEvent, 1)
	b.clients[ch] = true
	return ch, b.disabled, true
}

// Broadcast sends a navigation event to all connected clients.
func (b *Broadcaster) Broadcast(ctx context.Context, url string) {
	event := NavigationEvent{
		URL:       url,
		Timestamp: time.Now(),
	}

	b.mu.RLock()
	defer b.mu.RUnlock()
	clientCount := len(b.clients)

	if clientCount == 0 {
		b.logger.Debug("No clients connected for navigation broadcast", zap.String("url", url))
		return
	}

	sentCount := 0
	for ch := range b.clients {
		select {
		case ch <- event:
			sentCount++
		case <-ctx.Done():
			return
		default:
			// Client channel is full, skip (shouldn't happen with buffered channel)
			b.logger.Warn("Client channel full, skipping broadcast", zap.String("url", url))
		}
	}
}

// Subscribe adds a new client and returns their event channel.
func (b *Broadcaster) Subscribe() chan NavigationEvent {
	ch := make(chan NavigationEvent, 1)
	b.mu.Lock()
	b.clients[ch] = true
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a client.
func (b *Broadcaster) Unsubscribe(ch chan NavigationEvent) {
	b.mu.Lock()
	delete(b.clients, ch)
	close(ch)
	b.mu.Unlock()
}
