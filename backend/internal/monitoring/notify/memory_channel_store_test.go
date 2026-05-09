package notify

import (
	"fmt"
	"sync"
	"time"
)

type NotificationChannelStore struct {
	mu       sync.RWMutex
	channels []NotificationChannelConfig
}

func NewNotificationChannelStore(_ string) (*NotificationChannelStore, error) {
	return &NotificationChannelStore{}, nil
}

func (s *NotificationChannelStore) List() []NotificationChannelConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]NotificationChannelConfig, len(s.channels))
	for i, ch := range s.channels {
		out[i] = ch.SafeView()
	}
	return out
}

func (s *NotificationChannelStore) ListRaw() []NotificationChannelConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]NotificationChannelConfig, len(s.channels))
	copy(out, s.channels)
	return out
}

func (s *NotificationChannelStore) Get(id string) (NotificationChannelConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, ch := range s.channels {
		if ch.ID == id {
			return ch.SafeView(), true
		}
	}
	return NotificationChannelConfig{}, false
}

func (s *NotificationChannelStore) Create(ch NotificationChannelConfig) error {
	if err := ch.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.channels {
		if existing.ID == ch.ID {
			return fmt.Errorf("channel with id %q already exists", ch.ID)
		}
	}
	if ch.ID == "" {
		ch.ID = fmt.Sprintf("ch-%d", time.Now().UnixNano())
	}
	now := time.Now().UTC()
	ch.CreatedAt = now
	ch.UpdatedAt = now
	s.channels = append(s.channels, ch)
	return nil
}

func (s *NotificationChannelStore) Update(id string, ch NotificationChannelConfig) error {
	if err := ch.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i, existing := range s.channels {
		if existing.ID == id {
			ch.ID = id
			ch.CreatedAt = existing.CreatedAt
			ch.UpdatedAt = time.Now().UTC()
			if ch.SMTPPass == "••••••••" {
				ch.SMTPPass = existing.SMTPPass
			}
			if ch.BotToken != "" && len(ch.BotToken) > 4 && ch.BotToken[4:8] == "••••" {
				ch.BotToken = existing.BotToken
			}
			if ch.RoutingKey != "" && len(ch.RoutingKey) > 4 && ch.RoutingKey[4:8] == "••••" {
				ch.RoutingKey = existing.RoutingKey
			}
			s.channels[i] = ch
			return nil
		}
	}
	return fmt.Errorf("channel not found: %s", id)
}

func (s *NotificationChannelStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, ch := range s.channels {
		if ch.ID == id {
			s.channels = append(s.channels[:i], s.channels[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("channel not found: %s", id)
}

func (s *NotificationChannelStore) ToggleEnabled(id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.channels {
		if s.channels[i].ID == id {
			s.channels[i].Enabled = enabled
			s.channels[i].UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return fmt.Errorf("channel not found: %s", id)
}
