// Package playback contains the transport-independent core used to synchronize
// playback state between a user's WebSocket sessions.
package playback

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

const DefaultMaxMessageBytes = 64 * 1024

var (
	ErrInvalidUserID        = errors.New("playback user id is required")
	ErrInvalidSessionID     = errors.New("playback session id is required")
	ErrSessionNotRegistered = errors.New("playback session is not registered")
	ErrMessageTooLarge      = errors.New("playback message is too large")
)

// Session is one connected playback client. Messages exposes the outbound
// states that a WebSocket writer should send to the client. The channel is
// closed when the session is unregistered or replaced by another connection
// with the same user and session IDs.
//
// A session has a one-element, latest-state queue. If its writer is slow, a new
// state replaces the queued stale state instead of blocking every other user
// session.
type Session struct {
	userID string
	id     string
	send   chan []byte
}

func (s *Session) UserID() string {
	if s == nil {
		return ""
	}
	return s.userID
}

func (s *Session) ID() string {
	if s == nil {
		return ""
	}
	return s.id
}

func (s *Session) Messages() <-chan []byte {
	if s == nil {
		return nil
	}
	return s.send
}

// Hub groups active sessions by authenticated user and retains the most recent
// playback state for each user. It is safe for concurrent use.
type Hub struct {
	mu              sync.Mutex
	maxMessageBytes int
	sessions        map[string]map[string]*Session
	lastState       map[string][]byte
}

// NewHub creates a playback hub. A non-positive limit selects
// DefaultMaxMessageBytes.
func NewHub(maxMessageBytes int) *Hub {
	if maxMessageBytes <= 0 {
		maxMessageBytes = DefaultMaxMessageBytes
	}
	return &Hub{
		maxMessageBytes: maxMessageBytes,
		sessions:        make(map[string]map[string]*Session),
		lastState:       make(map[string][]byte),
	}
}

func (h *Hub) MaxMessageBytes() int {
	if h == nil {
		return 0
	}
	return h.maxMessageBytes
}

// Register adds one authenticated user session. If the user already has a
// last-known state, it is immediately queued for replay. Re-registering the
// same user/session pair replaces and closes the previous session.
func (h *Hub) Register(userID, sessionID string) (*Session, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, ErrInvalidUserID
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, ErrInvalidSessionID
	}

	session := &Session{
		userID: userID,
		id:     sessionID,
		send:   make(chan []byte, 1),
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	userSessions := h.sessions[userID]
	if userSessions == nil {
		userSessions = make(map[string]*Session)
		h.sessions[userID] = userSessions
	}
	if previous := userSessions[sessionID]; previous != nil {
		close(previous.send)
	}
	userSessions[sessionID] = session

	if state, ok := h.lastState[userID]; ok {
		session.send <- clone(state)
	}
	return session, nil
}

// Unregister removes a session and closes its outbound message channel. It
// returns false if the session is nil, stale, or was already removed.
func (h *Hub) Unregister(session *Session) bool {
	if h == nil || session == nil {
		return false
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	userSessions := h.sessions[session.userID]
	if userSessions == nil || userSessions[session.id] != session {
		return false
	}
	delete(userSessions, session.id)
	if len(userSessions) == 0 {
		delete(h.sessions, session.userID)
	}
	close(session.send)
	return true
}

// Broadcast records a state received from sender and queues it for every other
// active session belonging to the same user. The sender is excluded because it
// already owns this state. The returned value is the number of target sessions.
func (h *Hub) Broadcast(sender *Session, message []byte) (int, error) {
	if h == nil || sender == nil {
		return 0, ErrSessionNotRegistered
	}
	if err := h.validateMessage(message); err != nil {
		return 0, err
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	userSessions := h.sessions[sender.userID]
	if userSessions == nil || userSessions[sender.id] != sender {
		return 0, ErrSessionNotRegistered
	}
	h.lastState[sender.userID] = clone(message)

	recipients := 0
	for _, session := range userSessions {
		if session == sender {
			continue
		}
		enqueueLatest(session.send, message)
		recipients++
	}
	return recipients, nil
}

// BroadcastToUser records a server-originated state and queues it for all
// active sessions of userID. It is useful when state changes outside a
// particular WebSocket connection.
func (h *Hub) BroadcastToUser(userID string, message []byte) (int, error) {
	if h == nil {
		return 0, ErrInvalidUserID
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return 0, ErrInvalidUserID
	}
	if err := h.validateMessage(message); err != nil {
		return 0, err
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.lastState[userID] = clone(message)
	userSessions := h.sessions[userID]
	for _, session := range userSessions {
		enqueueLatest(session.send, message)
	}
	return len(userSessions), nil
}

// LastState returns a defensive copy of the latest state for a user.
func (h *Hub) LastState(userID string) ([]byte, bool) {
	if h == nil {
		return nil, false
	}
	userID = strings.TrimSpace(userID)
	h.mu.Lock()
	defer h.mu.Unlock()

	state, ok := h.lastState[userID]
	if !ok {
		return nil, false
	}
	return clone(state), true
}

// SessionCount reports the current number of active sessions for a user.
func (h *Hub) SessionCount(userID string) int {
	if h == nil {
		return 0
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.sessions[strings.TrimSpace(userID)])
}

func (h *Hub) validateMessage(message []byte) error {
	if len(message) > h.maxMessageBytes {
		return fmt.Errorf("%w: got %d bytes, limit is %d", ErrMessageTooLarge, len(message), h.maxMessageBytes)
	}
	return nil
}

// enqueueLatest never blocks. Only Hub calls it, while holding h.mu, so the
// channel cannot be closed or written by another hub goroutine concurrently.
func enqueueLatest(destination chan []byte, message []byte) {
	copyOfMessage := clone(message)
	select {
	case destination <- copyOfMessage:
		return
	default:
	}

	// Drop the queued stale state. A concurrent receiver may have consumed it
	// already, hence both operations remain non-blocking.
	select {
	case <-destination:
	default:
	}
	select {
	case destination <- copyOfMessage:
	default:
	}
}

func clone(message []byte) []byte {
	return append([]byte(nil), message...)
}
