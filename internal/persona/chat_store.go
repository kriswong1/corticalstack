package persona

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

const sessionTTL = 30 * time.Minute

type sessionEntry struct {
	session   *ChatSession
	expiresAt time.Time
	// busy is set while a handler holds an exclusive lease on the
	// session (via Acquire) and cleared by Release. Guarded by the
	// enclosing ChatStore.mu — the bit itself is not atomic.
	busy bool
}

// ChatStore is a thread-safe in-memory store for active persona chat
// sessions. Sessions expire after 30 minutes of inactivity. Call Close
// to stop the background reaper — required for clean test teardown.
type ChatStore struct {
	mu       sync.Mutex
	sessions map[string]*sessionEntry

	stopOnce sync.Once
	stop     chan struct{}
}

// NewChatStore creates an empty store and starts the reaper goroutine.
func NewChatStore() *ChatStore {
	s := &ChatStore{
		sessions: make(map[string]*sessionEntry),
		stop:     make(chan struct{}),
	}
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
// Prefer Acquire when the caller is about to mutate session state.
func (s *ChatStore) Get(id string) *ChatSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.sessions[id]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil
	}
	return entry.session
}

// Touch resets the TTL for a session.
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

// MustGet retrieves a session or returns an error. Read-only callers
// (e.g. AcceptPersonaChat snapshotting Result before delete) still use
// this; mutating callers should use Acquire/Release instead so concurrent
// Continue/Finish requests don't race on session.Messages.
func (s *ChatStore) MustGet(id string) (*ChatSession, error) {
	session := s.Get(id)
	if session == nil {
		return nil, fmt.Errorf("session not found or expired: %s", id)
	}
	return session, nil
}

// Acquire takes an exclusive lease on a session for the duration of a
// mutation (Continue or Finish). Returns the session; a second caller
// for the same ID while the first hasn't Released gets status=409 with
// a Conflict error. Returns status=404 when the session is missing or
// expired. status=0 on success.
//
// Pair every successful Acquire with a deferred Release — the reaper
// will not evict a busy entry, so a leaked Acquire permanently pins
// the session in the map.
func (s *ChatStore) Acquire(id string) (*ChatSession, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.sessions[id]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, http.StatusNotFound, fmt.Errorf("session not found or expired: %s", id)
	}
	if entry.busy {
		return nil, http.StatusConflict, fmt.Errorf("session %s is already processing a request", id)
	}
	entry.busy = true
	// Extend the TTL while the request runs so a long Claude call
	// doesn't get reaped mid-flight.
	entry.expiresAt = time.Now().Add(sessionTTL)
	return entry.session, 0, nil
}

// Release clears the busy bit set by Acquire. Safe to call for an
// unknown id (the reaper may have run between Acquire and Release if
// TTL got twiddled by hand in tests).
func (s *ChatStore) Release(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.sessions[id]; ok {
		entry.busy = false
	}
}

// Close stops the reaper goroutine. Idempotent. After Close the store
// remains usable for in-memory CRUD; only the background TTL reaper
// has stopped.
func (s *ChatStore) Close() {
	s.stopOnce.Do(func() { close(s.stop) })
}

// reapLoop periodically removes expired sessions. Exits when Close is
// called. Never evicts an entry that is currently busy — otherwise a
// deferred Release would operate on a ghost session.
func (s *ChatStore) reapLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now()
			for id, entry := range s.sessions {
				if entry.busy {
					continue
				}
				if now.After(entry.expiresAt) {
					delete(s.sessions, id)
				}
			}
			s.mu.Unlock()
		}
	}
}
