package mcpapi

import (
	"sync"
	"time"

	"github.com/google/uuid"

	"atlas/internal/identity"
)

const (
	sessionTTL      = time.Hour
	maxSessions     = 1024
	sessionIDHeader = "MCP-Session-Id"
)

type mcpSession struct {
	organizationID string
	connectionID   string
	expiresAt      time.Time
}

// sessionStore keeps optional in-process MCP sessions. The endpoint remains
// usable stateless; clients that send MCP-Session-Id get binding and DELETE
// termination. Sessions are per process, so deployments behind multiple
// instances should use sticky routing or stay stateless.
type sessionStore struct {
	mutex    sync.Mutex
	sessions map[string]mcpSession
	now      func() time.Time
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: map[string]mcpSession{}, now: time.Now}
}

func (s *sessionStore) create(principal identity.Principal) string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	now := s.now()
	s.reapLocked(now)
	if len(s.sessions) >= maxSessions {
		for id := range s.sessions {
			delete(s.sessions, id)
			break
		}
	}
	id := uuid.NewString()
	s.sessions[id] = mcpSession{
		organizationID: principal.OrganizationID,
		connectionID:   principal.ID,
		expiresAt:      now.Add(sessionTTL),
	}
	return id
}

func (s *sessionStore) validate(id string, principal identity.Principal) (bool, bool) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return false, false
	}
	if s.now().After(session.expiresAt) {
		delete(s.sessions, id)
		return false, false
	}
	if session.organizationID != principal.OrganizationID || session.connectionID != principal.ID {
		return true, false
	}
	session.expiresAt = s.now().Add(sessionTTL)
	s.sessions[id] = session
	return true, true
}

func (s *sessionStore) delete(id string) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if _, ok := s.sessions[id]; !ok {
		return false
	}
	delete(s.sessions, id)
	return true
}

func (s *sessionStore) reapLocked(now time.Time) {
	for id, session := range s.sessions {
		if now.After(session.expiresAt) {
			delete(s.sessions, id)
		}
	}
}
