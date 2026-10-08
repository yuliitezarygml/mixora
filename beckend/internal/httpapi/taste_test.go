package httpapi

import (
	"github.com/iulian/soundcloud-go/internal/library"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTasteAPIRequiresOwnerSessionAndPersistsCompletion(t *testing.T) {
	fixture := newContractFixture()
	server := httptest.NewServer(fixture.handler())
	defer server.Close()
	response := request(t, server.Client(), http.MethodGet, server.URL+"/api/v1/me/taste", nil)
	assertStatus(t, response, http.StatusUnauthorized)
	closeResponse(t, response)
	req := newJSONRequest(t, http.MethodPut, server.URL+"/api/v1/me/taste", map[string]any{"artists": []string{"One", "Two", "Three", "Four", "Five"}, "genres": []string{"rock"}})
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: fixture.sessionToken})
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, response, http.StatusOK)
	var saved library.TasteProfile
	decodeResponse(t, response, &saved)
	if !saved.Completed || len(saved.Artists) != 5 {
		t.Fatal("profile not completed")
	}
	if len(fixture.library.tastes) != 1 || !fixture.library.tastes[fixture.user.ID].Completed {
		t.Fatal("wrong profile owner")
	}
	other, _ := fixture.library.GetTaste(t.Context(), "other-account")
	if other.Completed {
		t.Fatal("profile leaked between accounts")
	}
}
