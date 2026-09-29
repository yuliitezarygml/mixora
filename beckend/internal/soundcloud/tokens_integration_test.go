package soundcloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"mixora/beckend/internal/database"
)

func TestTokenPersistenceAndConcurrentRotation(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL for token storage tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("mixora_tokens_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(dsn)
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
	var exchanges atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := exchanges.Add(1)
		r.ParseForm()
		if n == 2 && r.Form.Get("refresh_token") != "refresh-1" {
			t.Error("refresh token was not retained")
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": fmt.Sprintf("access-%d", n), "refresh_token": fmt.Sprintf("refresh-%d", n), "expires_in": 3600})
	}))
	defer remote.Close()
	create := func() *SQLTokens {
		store, err := NewTokens(db, "client", "secret", strings.Repeat("ab", 32))
		if err != nil {
			t.Fatal(err)
		}
		store.endpoint = remote.URL
		return store
	}
	a, b := create(), create()
	if token, err := a.AccessToken(ctx); err != nil || token != "access-1" {
		t.Fatalf("initial: %q %v", token, err)
	}
	if token, err := b.AccessToken(ctx); err != nil || token != "access-1" {
		t.Fatalf("persistent: %q %v", token, err)
	}
	if exchanges.Load() != 1 {
		t.Fatal("token unnecessarily reissued")
	}
	expired, _ := json.Marshal(Token{AccessToken: "access-1", RefreshToken: "refresh-1", ExpiresAt: time.Now().Add(-time.Minute)})
	encrypted, err := a.seal(expired)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "UPDATE provider_tokens SET encrypted_token=$1", encrypted); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store := a
			if i%2 == 0 {
				store = b
			}
			token, err := store.AccessToken(ctx)
			if err != nil || token != "access-2" {
				t.Errorf("concurrent refresh: %q %v", token, err)
			}
		}(i)
	}
	wg.Wait()
	if exchanges.Load() != 2 {
		t.Fatalf("single-use token refreshed %d times", exchanges.Load()-1)
	}
}
