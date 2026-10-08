package recommendation

import (
	"context"
	"testing"

	"github.com/iulian/soundcloud-go/internal/music"
)

type tasteStub struct{}

func (tasteStub) RecommendationTaste(context.Context, string) ([]string, []string, error) {
	return []string{"Chosen", "Other"}, []string{"electronic"}, nil
}
func TestStoredTasteChangesColdStartWithoutFabricatedLikes(t *testing.T) {
	catalog := &candidateCatalogStub{tracks: []music.Track{{ID: "one", Source: "youtube", Artist: "Random"}, {ID: "two", Source: "bandcamp", Artist: "Chosen", Genre: "Electronic"}}}
	service := New(nil, WithCatalog(catalog), WithTasteSource(tasteStub{}), WithCollaborative(collaborativeStub{ids: []string{"youtube:one"}}))
	result, err := service.Recommend(context.Background(), Request{UserID: "listener", Explicit: true})
	if err != nil || result.Reason != "Chosen" || result.Tracks[0].ID != "two" {
		t.Fatalf("cold start=%#v err=%v", result, err)
	}
	next, err := service.Recommend(context.Background(), Request{UserID: "listener", Round: 1, Explicit: true})
	if err != nil || next.Reason != "Other" {
		t.Fatalf("artist rotation=%#v", next)
	}
	explicit, err := service.Recommend(context.Background(), Request{UserID: "listener", Context: Context{Artist: "Station"}, Explicit: true})
	if err != nil || explicit.Reason != "Station" {
		t.Fatal("profile overwrote explicit station context")
	}
}
