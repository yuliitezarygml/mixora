package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"mixora/beckend/internal/catalog"
	"mixora/beckend/internal/config"
	"mixora/beckend/internal/database"
)

// Each run uses its own schema and never truncates the development database.
func TestIntegration(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to test against PostgreSQL")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("mixora_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err = database.Migrate(ctx, db); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
	media := t.TempDir()
	audio := []byte("RIFF1234567890WAVE-test-audio")
	if err = os.WriteFile(filepath.Join(media, "test.wav"), audio, 0644); err != nil {
		t.Fatal(err)
	}
	track, err := catalog.New(db).Create(ctx, catalog.Track{Title: "Оригинал", Artist: "Mixora", Explicit: true, MediaKey: "test.wav"})
	if err != nil {
		t.Fatal(err)
	}
	api, err := New(db, config.Config{MediaDir: media, AllowedOrigin: "http://localhost:5173"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	handler := api.Handler()
	request := func(method, path, body string, cookie *http.Cookie, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, status, w.Body.String())
		}
		return w
	}
	request("GET", "/health/ready", "", nil, 200)
	request("GET", "/api/v1/me", "", nil, 401)
	request("POST", "/api/v1/auth/register", `{"email":"not-email","password":"weak","display_name":"A"}`, nil, 400)
	for _, name := range []string{"alice", "bob"} {
		request("POST", "/api/v1/auth/register", fmt.Sprintf(`{"email":"%s@example.test","password":"correct-password-123","display_name":"%s"}`, name, name), nil, 201)
	}
	request("POST", "/api/v1/auth/register", `{"email":"ALICE@example.test","password":"correct-password-123","display_name":"Alice"}`, nil, 409)
	request("POST", "/api/v1/auth/login", `{"email":"alice@example.test","password":"wrong"}`, nil, 401)
	var aliceToken, bobToken string
	login := func(email string) *http.Cookie {
		w := request("POST", "/api/v1/auth/login", fmt.Sprintf(`{"email":"%s@example.test","password":"correct-password-123"}`, email), nil, 200)
		cookies := w.Result().Cookies()
		if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
			t.Fatal("unsafe session cookie")
		}
		var body struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || len(body.Token) != 64 || body.Token != cookies[0].Value {
			t.Fatalf("login must return the account token: %s", w.Body.String())
		}
		if email == "alice" {
			aliceToken = body.Token
		} else {
			bobToken = body.Token
		}
		return cookies[0]
	}
	alice, bob := login("alice"), login("bob")
	resumed := request("POST", "/api/v1/auth/resume", fmt.Sprintf(`{"token":"%s"}`, aliceToken), nil, 200)
	if got := resumed.Result().Cookies(); len(got) != 1 || got[0].Value != aliceToken {
		t.Fatal("resume did not restore the saved account token")
	}
	if me := request("GET", "/api/v1/me", "", resumed.Result().Cookies()[0], 200); !bytes.Contains(me.Body.Bytes(), []byte("alice@example.test")) {
		t.Fatal("resumed token opened the wrong account")
	}
	request("POST", "/api/v1/auth/resume", `{"token":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`, nil, 401)
	if session := request("GET", "/api/v1/auth/session", "", alice, 200); !bytes.Contains(session.Body.Bytes(), []byte(aliceToken)) {
		t.Fatal("active session token was not returned")
	}
	request("GET", "/api/v1/auth/session", "", nil, 401)
	result := request("GET", "/api/v1/me", "", alice, 200)
	if !bytes.Contains(result.Body.Bytes(), []byte(`"plus":false`)) {
		t.Fatal("new account must not have a subscription")
	}
	request("POST", "/api/v1/me/subscription", `{"plus":true}`, alice, 200)
	result = request("GET", "/api/v1/me", "", alice, 200)
	if !bytes.Contains(result.Body.Bytes(), []byte(`"plus":true`)) {
		t.Fatal("subscription was not saved")
	}
	result = request("GET", "/api/v1/me", "", bob, 200)
	if bytes.Contains(result.Body.Bytes(), []byte(`"plus":true`)) {
		t.Fatal("subscription leaked to another account")
	}
	result = request("GET", "/api/v1/tracks?q=Оригинал", "", nil, 200)
	if !bytes.Contains(result.Body.Bytes(), []byte(`"explicit":true`)) {
		t.Fatal("explicit track hidden")
	}
	if bytes.Contains(result.Body.Bytes(), []byte("test.wav")) {
		t.Fatal("media key exposed")
	}
	request("GET", "/api/v1/tracks?limit=0", "", nil, 400)
	request("GET", "/api/v1/tracks/not-a-uuid", "", nil, 400)
	request("GET", "/api/v1/tracks/"+track.ID+"/stream", "", nil, 401)
	r := httptest.NewRequest("GET", "/api/v1/tracks/"+track.ID+"/stream", nil)
	r.AddCookie(alice)
	r.Header.Set("Range", "bytes=4-9")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 206 || w.Body.String() != string(audio[4:10]) || w.Header().Get("Content-Range") == "" {
		t.Fatalf("range playback failed: %d %q", w.Code, w.Body.String())
	}
	result = request("POST", "/api/v1/playlists", `{"name":"Мои треки"}`, alice, 201)
	var p struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(result.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/playlists/" + p.ID
	body := fmt.Sprintf(`{"track_id":%q}`, track.ID)
	request("POST", path+"/tracks", body, alice, 204)
	request("POST", path+"/tracks", body, alice, 204)
	result = request("GET", path+"/tracks", "", alice, 200)
	var list struct {
		Items []catalog.Track `json:"items"`
	}
	json.Unmarshal(result.Body.Bytes(), &list)
	if len(list.Items) != 1 {
		t.Fatal("duplicate playlist entry")
	}
	request("GET", path+"/tracks", "", bob, 404)
	request("POST", path+"/tracks", body, bob, 404)
	request("DELETE", path+"/tracks/"+track.ID, "", bob, 404)
	request("DELETE", path, "", bob, 404)
	request("GET", "/api/v1/providers/soundcloud/tracks?q=music", "", alice, 503)
	request("POST", "/api/v1/playlists", `{"name":"x","user_id":"bob"}`, alice, 400)
	r = httptest.NewRequest("POST", "/api/v1/playlists", strings.NewReader(`{"name":"x"}`))
	r.AddCookie(alice)
	r.Header.Set("Origin", "https://evil.example")
	r.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-origin write accepted")
	}
	// A malicious stored path or symlink must never expose files outside storage.
	outside := filepath.Join(t.TempDir(), "private.wav")
	os.WriteFile(outside, []byte("private"), 0600)
	os.Symlink(outside, filepath.Join(media, "escape.wav"))
	bad, err := catalog.New(db).Create(ctx, catalog.Track{Title: "bad", Artist: "test", MediaKey: "escape.wav"})
	if err != nil {
		t.Fatal(err)
	}
	result = request("GET", "/api/v1/tracks/"+bad.ID+"/stream", "", alice, 500)
	if strings.Contains(result.Body.String(), "private") {
		t.Fatal("file escaped storage root")
	}
	request("DELETE", path+"/tracks/"+track.ID, "", alice, 204)
	request("DELETE", path, "", alice, 204)
	request("GET", "/api/v1/library", "", nil, 401)
	result = request("GET", "/api/v1/library", "", alice, 200)
	if !bytes.Contains(result.Body.Bytes(), []byte(`"likes":[]`)) {
		t.Fatal("empty library missing")
	}
	request("PUT", "/api/v1/library", `{"likes":[{"id":"1","source":"soundcloud","title":"Утро","artist":"Дайте танк"}]}`, alice, 204)
	result = request("GET", "/api/v1/library", "", alice, 200)
	if !bytes.Contains(result.Body.Bytes(), []byte("Утро")) {
		t.Fatal("library not saved")
	}
	request("PUT", "/api/v1/library", `{"pins":["playlist-1"]}`, alice, 204)
	result = request("GET", "/api/v1/library", "", alice, 200)
	if !bytes.Contains(result.Body.Bytes(), []byte(`"playlist-1"`)) {
		t.Fatal("pinned playlist was not saved")
	}
	result = request("GET", "/api/v1/library", "", bob, 200)
	if bytes.Contains(result.Body.Bytes(), []byte("Утро")) {
		t.Fatal("library leaked between accounts")
	}
	tooMany := `{"likes":[` + strings.TrimRight(strings.Repeat(`{"id":"x"},`, 401), ",") + `]}`
	request("PUT", "/api/v1/library", tooMany, alice, 400)
	result = request("POST", "/api/v1/wave", `{"preferences":{"diversity":"familiar","mood":"any","language":"any"},"likes":[{"id":"1","source":"soundcloud","title":"Утро","artist":"Дайте танк","access":"playable"}]}`, alice, 200)
	if !bytes.Contains(result.Body.Bytes(), []byte("Утро")) {
		t.Fatal("familiar wave ignored saved seeds")
	}
	request("POST", "/api/v1/wave", `{"preferences":{"diversity":"balanced","mood":"any","language":"any"}}`, nil, 401)
	request("POST", "/api/v1/wave", `{"preferences":{"diversity":"balanced","mood":"any","language":"any"}}`, alice, 503)
	request("GET", "/api/v1/playback/ws", "", nil, 401)
	socketServer := httptest.NewServer(handler)
	defer socketServer.Close()
	dial := func(cookie *http.Cookie) *websocket.Conn {
		t.Helper()
		conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(socketServer.URL, "http")+"/api/v1/playback/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Cookie": []string{cookie.Name + "=" + cookie.Value}, "Origin": []string{"http://localhost:5173"}}})
		if err != nil {
			t.Fatal(err)
		}
		return conn
	}
	phone, other := dial(alice), dial(alice)
	stranger := dial(bob)
	defer phone.Close(websocket.StatusNormalClosure, "")
	defer other.Close(websocket.StatusNormalClosure, "")
	defer stranger.Close(websocket.StatusNormalClosure, "")
	if err = phone.Write(ctx, websocket.MessageText, []byte(`{"type":"state","playing":true,"position":1,"track":{"id":"1","title":"Утро"}}`)); err != nil {
		t.Fatal(err)
	}
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	_, payload, readErr := other.Read(readCtx)
	cancel()
	if readErr != nil || !bytes.Contains(payload, []byte("Утро")) {
		t.Fatalf("player state did not reach the other device: %v %s", readErr, payload)
	}
	ownCtx, ownCancel := context.WithTimeout(ctx, 200*time.Millisecond)
	_, echoed, readErr := phone.Read(ownCtx)
	ownCancel()
	if readErr == nil {
		t.Fatalf("player state echoed to the sender: %s", echoed)
	}
	strangerCtx, strangerCancel := context.WithTimeout(ctx, 200*time.Millisecond)
	_, leaked, readErr := stranger.Read(strangerCtx)
	strangerCancel()
	if readErr == nil {
		t.Fatalf("player state leaked to another account: %s", leaked)
	}
	request("POST", "/api/v1/auth/logout", "", alice, 204)
	request("GET", "/api/v1/me", "", alice, 401)
	request("POST", "/api/v1/auth/resume", fmt.Sprintf(`{"token":"%s"}`, aliceToken), nil, 401)
	if bobBack := request("POST", "/api/v1/auth/resume", fmt.Sprintf(`{"token":"%s"}`, bobToken), nil, 200); !bytes.Contains(bobBack.Body.Bytes(), []byte("bob@example.test")) {
		t.Fatal("the other account token was dropped on logout")
	}
	if _, err = db.Exec(ctx, "UPDATE sessions SET expires_at=now()-interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	request("GET", "/api/v1/me", "", bob, 401)
}
