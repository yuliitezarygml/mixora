package playback

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestBroadcastStaysWithinUserAndExcludesSender(t *testing.T) {
	t.Parallel()
	hub := NewHub(1024)
	sender := mustRegister(t, hub, "user-a", "desktop")
	receiver := mustRegister(t, hub, "user-a", "phone")
	otherUser := mustRegister(t, hub, "user-b", "phone")

	payload := []byte(`{"type":"state","playing":true}`)
	recipients, err := hub.Broadcast(sender, payload)
	if err != nil {
		t.Fatalf("Broadcast() error = %v", err)
	}
	if recipients != 1 {
		t.Fatalf("Broadcast() recipients = %d, want 1", recipients)
	}
	assertMessage(t, receiver.Messages(), payload)
	assertNoMessage(t, sender.Messages())
	assertNoMessage(t, otherUser.Messages())
}

func TestRegisterReplaysLastStateAndCopiesPayload(t *testing.T) {
	t.Parallel()
	hub := NewHub(1024)
	sender := mustRegister(t, hub, "user-a", "desktop")
	payload := []byte(`{"type":"state","position":42}`)

	if _, err := hub.Broadcast(sender, payload); err != nil {
		t.Fatalf("Broadcast() error = %v", err)
	}
	payload[0] = 'x'
	if !hub.Unregister(sender) {
		t.Fatal("Unregister() sender = false, want true")
	}

	receiver := mustRegister(t, hub, "user-a", "phone")
	want := []byte(`{"type":"state","position":42}`)
	replayed := receive(t, receiver.Messages())
	if !bytes.Equal(replayed, want) {
		t.Fatalf("replayed state = %q, want %q", replayed, want)
	}

	replayed[0] = 'y'
	stored, ok := hub.LastState("user-a")
	if !ok || !bytes.Equal(stored, want) {
		t.Fatalf("LastState() = %q, %v; want %q, true", stored, ok, want)
	}
}

func TestMessageLimitRejectsOversizeWithoutReplacingState(t *testing.T) {
	t.Parallel()
	hub := NewHub(5)
	sender := mustRegister(t, hub, "user-a", "desktop")

	if _, err := hub.Broadcast(sender, []byte("12345")); err != nil {
		t.Fatalf("Broadcast() at limit error = %v", err)
	}
	if _, err := hub.Broadcast(sender, []byte("123456")); !errors.Is(err, ErrMessageTooLarge) {
		t.Fatalf("Broadcast() oversized error = %v, want ErrMessageTooLarge", err)
	}
	state, ok := hub.LastState("user-a")
	if !ok || string(state) != "12345" {
		t.Fatalf("LastState() = %q, %v; want %q, true", state, ok, "12345")
	}
}

func TestLatestStateReplacesQueuedState(t *testing.T) {
	t.Parallel()
	hub := NewHub(1024)
	sender := mustRegister(t, hub, "user-a", "desktop")
	receiver := mustRegister(t, hub, "user-a", "phone")

	if _, err := hub.Broadcast(sender, []byte("old")); err != nil {
		t.Fatalf("first Broadcast() error = %v", err)
	}
	if _, err := hub.Broadcast(sender, []byte("new")); err != nil {
		t.Fatalf("second Broadcast() error = %v", err)
	}
	assertMessage(t, receiver.Messages(), []byte("new"))
	assertNoMessage(t, receiver.Messages())
}

func TestUnregisterClosesSessionAndRejectsStaleSender(t *testing.T) {
	t.Parallel()
	hub := NewHub(1024)
	session := mustRegister(t, hub, "user-a", "desktop")

	if !hub.Unregister(session) {
		t.Fatal("Unregister() = false, want true")
	}
	if hub.Unregister(session) {
		t.Fatal("second Unregister() = true, want false")
	}
	if _, open := <-session.Messages(); open {
		t.Fatal("session message channel remains open")
	}
	if _, err := hub.Broadcast(session, []byte("state")); !errors.Is(err, ErrSessionNotRegistered) {
		t.Fatalf("Broadcast() stale session error = %v, want ErrSessionNotRegistered", err)
	}
	if got := hub.SessionCount("user-a"); got != 0 {
		t.Fatalf("SessionCount() = %d, want 0", got)
	}
}

func TestDuplicateRegistrationReplacesOldSession(t *testing.T) {
	t.Parallel()
	hub := NewHub(1024)
	oldSession := mustRegister(t, hub, "user-a", "desktop")
	newSession := mustRegister(t, hub, "user-a", "desktop")

	if _, open := <-oldSession.Messages(); open {
		t.Fatal("replaced session message channel remains open")
	}
	if hub.Unregister(oldSession) {
		t.Fatal("Unregister() removed a newer session through stale handle")
	}
	if got := hub.SessionCount("user-a"); got != 1 {
		t.Fatalf("SessionCount() = %d, want 1", got)
	}
	if _, err := hub.Broadcast(newSession, []byte("state")); err != nil {
		t.Fatalf("Broadcast() from replacement error = %v", err)
	}
}

func TestBroadcastToUserReachesAllSessionsAndReplays(t *testing.T) {
	t.Parallel()
	hub := NewHub(1024)
	first := mustRegister(t, hub, "user-a", "desktop")
	second := mustRegister(t, hub, "user-a", "phone")

	recipients, err := hub.BroadcastToUser("user-a", []byte("server-state"))
	if err != nil {
		t.Fatalf("BroadcastToUser() error = %v", err)
	}
	if recipients != 2 {
		t.Fatalf("BroadcastToUser() recipients = %d, want 2", recipients)
	}
	assertMessage(t, first.Messages(), []byte("server-state"))
	assertMessage(t, second.Messages(), []byte("server-state"))

	third := mustRegister(t, hub, "user-a", "tablet")
	assertMessage(t, third.Messages(), []byte("server-state"))
}

func TestInvalidRegistration(t *testing.T) {
	t.Parallel()
	hub := NewHub(0)
	if hub.MaxMessageBytes() != DefaultMaxMessageBytes {
		t.Fatalf("MaxMessageBytes() = %d, want %d", hub.MaxMessageBytes(), DefaultMaxMessageBytes)
	}
	if _, err := hub.Register(" ", "session"); !errors.Is(err, ErrInvalidUserID) {
		t.Fatalf("Register() blank user error = %v, want ErrInvalidUserID", err)
	}
	if _, err := hub.Register("user", " "); !errors.Is(err, ErrInvalidSessionID) {
		t.Fatalf("Register() blank session error = %v, want ErrInvalidSessionID", err)
	}
}

func TestHubConcurrentUse(t *testing.T) {
	hub := NewHub(1024)
	sender := mustRegister(t, hub, "user-a", "sender")

	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			session, err := hub.Register("user-a", fmt.Sprintf("session-%d", i))
			if err != nil {
				t.Errorf("Register() error = %v", err)
				return
			}
			_, _ = hub.Broadcast(sender, []byte("state"))
			hub.Unregister(session)
		}()
	}
	wg.Wait()
	if got := hub.SessionCount("user-a"); got != 1 {
		t.Fatalf("SessionCount() = %d, want sender only", got)
	}
}

func mustRegister(t *testing.T, hub *Hub, userID, sessionID string) *Session {
	t.Helper()
	session, err := hub.Register(userID, sessionID)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	return session
}

func assertMessage(t *testing.T, messages <-chan []byte, want []byte) {
	t.Helper()
	got := receive(t, messages)
	if !bytes.Equal(got, want) {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func receive(t *testing.T, messages <-chan []byte) []byte {
	t.Helper()
	select {
	case message, open := <-messages:
		if !open {
			t.Fatal("message channel closed")
		}
		return message
	default:
		t.Fatal("expected a queued message")
		return nil
	}
}

func assertNoMessage(t *testing.T, messages <-chan []byte) {
	t.Helper()
	select {
	case message, open := <-messages:
		if !open {
			t.Fatal("message channel unexpectedly closed")
		}
		t.Fatalf("unexpected message %q", message)
	default:
	}
}
