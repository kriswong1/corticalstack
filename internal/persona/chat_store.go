package persona

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

const sessionTTL = 30 * time.Minute

type sessionEntry struct {
	session   *ChatSession
	expiresAt time.Time
}

// ChatStore is a thread-safe in-memory store for active persona chat
// sessions. Sessions expire after 30 minutes of inactivity.
type ChatStore struct {
	mu       sync.RWMutex
	sessions map[string]*sessionEntry
}

// NewChatStore creates an empty store.
func NewChatStore() *ChatStore {
	s := &ChatStore{sessions: make(map[string]*sessionEntry)}
	go s.reapLoop()
	return s
}

// Put stores a session, assigning an ID if empty. Resets the TTL.
func (s *ChatStore) Put(session *ChatSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session.ID == "" {
		session.ID = uuid.NewString()
	}
	s.sessions[session.ID] = &sessionEntry{
		session:   session,
		expiresAt: time.Now().Add(sessionTTL),
	}
}

// Get retrieves a session by ID. Returns nil if not found or expired.
func (s *ChatStore) Get(id string) *ChatSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.sessions[id]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil
	}
	return entry.session
}

// Touch resets the TTL for a session (call after each interaction).
func (s *ChatStore) Touch(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.sessions[id]; ok {
		entry.expiresAt = time.Now().Add(sessionTTL)
	}
}

// Delete removes a session.
func (s *ChatStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

// MustGet retrieves a session or returns an error.
func (s *ChatStore) MustGet(id string) (*ChatSession, error) {
	session := s.Get(id)
	if session == nil {
		return nil, fmt.Errorf("session not found or expired: %s", id)
	}
	return session, nil
}

// reapLoop periodically removes expired sessions.
func (s *ChatStore) reapLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for id, entry := range s.sessions {
			if now.After(entry.expiresAt) {
				delete(s.sessions, id)
			}
		}
		s.mu.Unlock()
	}
}
