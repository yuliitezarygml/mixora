package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// playbackHub is Mixora's player room. Each signed-in user has one room.
// A device publishes player state; the other devices of that user receive it.
// Audio stays on the SoundCloud HLS request and does not travel through this socket.
type playbackHub struct {
	mu    sync.Mutex
	rooms map[string]map[*websocket.Conn]struct{}
}

func (h *playbackHub) join(userID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms == nil {
		h.rooms = map[string]map[*websocket.Conn]struct{}{}
	}
	if h.rooms[userID] == nil {
		h.rooms[userID] = map[*websocket.Conn]struct{}{}
	}
	h.rooms[userID][conn] = struct{}{}
}

func (h *playbackHub) leave(userID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rooms[userID], conn)
	if len(h.rooms[userID]) == 0 {
		delete(h.rooms, userID)
	}
}

func (h *playbackHub) publish(userID string, from *websocket.Conn, msg []byte) {
	h.mu.Lock()
	peers := make([]*websocket.Conn, 0, len(h.rooms[userID]))
	for conn := range h.rooms[userID] {
		if conn != from {
			peers = append(peers, conn)
		}
	}
	h.mu.Unlock()
	for _, conn := range peers {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := conn.Write(ctx, websocket.MessageText, msg)
		cancel()
		if err != nil {
			h.leave(userID, conn)
		}
	}
}

func (s *Server) playbackSocket(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("mixora_session")
	if err != nil {
		fail(w, 401, "authentication required")
		return
	}
	user, err := s.auth.Authenticate(r.Context(), cookie.Value)
	if err != nil {
		s.handleError(w, err)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// The page origin is 5174 and the API host is 8080. Middleware already
		// rejects a foreign Origin; the proxy strips the page origin before it arrives.
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	conn.SetReadLimit(64 << 10)
	defer conn.Close(websocket.StatusNormalClosure, "")
	s.playback.join(user.ID, conn)
	defer s.playback.leave(user.ID, conn)
	for {
		_, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		clean, ok := normalizePlaybackState(data)
		if !ok {
			continue
		}
		s.playback.publish(user.ID, conn, clean)
	}
}

func normalizePlaybackState(data []byte) ([]byte, bool) {
	var msg struct {
		Type     string            `json:"type"`
		Playing  bool              `json:"playing"`
		Position float64           `json:"position"`
		Track    json.RawMessage   `json:"track"`
		Queue    []json.RawMessage `json:"queue"`
	}
	if json.Unmarshal(data, &msg) != nil || msg.Type != "state" || len(msg.Track) == 0 || len(msg.Track) > 4096 {
		return nil, false
	}
	if len(msg.Queue) > 30 {
		msg.Queue = msg.Queue[:30]
	}
	for _, item := range msg.Queue {
		if len(item) > 4096 {
			return nil, false
		}
	}
	if msg.Position < 0 || msg.Position > 60*60*6 {
		msg.Position = 0
	}
	clean, err := json.Marshal(msg)
	return clean, err == nil
}
