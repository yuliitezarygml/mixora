package httpapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

const (
	websocketWriteWait = 10 * time.Second
	websocketPongWait  = 60 * time.Second
	websocketPingEvery = 45 * time.Second
)

func (s *Server) playbackWebSocket(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		HandshakeTimeout: 5 * time.Second,
		CheckOrigin: func(request *http.Request) bool {
			origin := request.Header.Get("Origin")
			if origin == "" {
				return true
			}
			parsed, err := url.Parse(origin)
			if err != nil {
				return false
			}
			if parsed.Host == request.Host {
				return true
			}
			public, err := url.Parse(s.publicURL)
			return err == nil && public.Host != "" && parsed.Host == public.Host
		},
	}
	connection, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer connection.Close()

	owner := principalFrom(r)
	hubSession, err := s.playback.Register(owner.User.ID, owner.Session.ID+":"+randomID())
	if err != nil {
		_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "session unavailable"), time.Now().Add(websocketWriteWait))
		return
	}
	defer s.playback.Unregister(hubSession)

	done := make(chan struct{})
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		ticker := time.NewTicker(websocketPingEvery)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case message, ok := <-hubSession.Messages():
				if !ok {
					_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "replaced"), time.Now().Add(websocketWriteWait))
					return
				}
				_ = connection.SetWriteDeadline(time.Now().Add(websocketWriteWait))
				if err := connection.WriteMessage(websocket.TextMessage, message); err != nil {
					return
				}
			case <-ticker.C:
				if err := connection.WriteControl(websocket.PingMessage, nil, time.Now().Add(websocketWriteWait)); err != nil {
					return
				}
			}
		}
	}()

	connection.SetReadLimit(int64(s.playback.MaxMessageBytes()))
	_ = connection.SetReadDeadline(time.Now().Add(websocketPongWait))
	connection.SetPongHandler(func(string) error {
		return connection.SetReadDeadline(time.Now().Add(websocketPongWait))
	})
	for {
		messageType, message, err := connection.ReadMessage()
		if err != nil {
			break
		}
		if messageType != websocket.TextMessage || !validPlaybackState(message) {
			_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseUnsupportedData, "invalid state"), time.Now().Add(websocketWriteWait))
			break
		}
		if _, err := s.playback.Broadcast(hubSession, message); err != nil {
			break
		}
	}
	close(done)
	_ = connection.Close()
	<-closed
}

func validPlaybackState(message []byte) bool {
	var header struct {
		Type  string          `json:"type"`
		Track json.RawMessage `json:"track"`
		Queue json.RawMessage `json:"queue"`
	}
	if err := json.Unmarshal(message, &header); err != nil {
		return false
	}
	return header.Type == "state" && len(header.Track) > 0 && len(header.Queue) <= 48*1024
}
