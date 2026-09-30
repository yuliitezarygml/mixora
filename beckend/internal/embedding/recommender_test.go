package embedding

import (
	"context"
	"testing"
)

type fakeContentStore struct {
	serverSeeds []string
	similar     []string
	nearest     []string
}

func (f fakeContentStore) TasteSeeds(context.Context, string, int) ([]string, error) {
	return f.serverSeeds, nil
}
func (f fakeContentStore) Similar(context.Context, []string, int) ([]string, error) {
	return f.similar, nil
}
func (f fakeContentStore) Nearest(context.Context, []float32, []string, int) ([]string, error) {
	return f.nearest, nil
}

func TestRecommenderCombinesTasteAndColdStartQuery(t *testing.T) {
	recommender, err := NewRecommender(fakeContentStore{
		serverSeeds: []string{"track:seed"},
		similar:     []string{"track:a", "track:b"},
		nearest:     []string{"track:b", "track:c"},
	}, fakeEmbedder{}, "embeddinggemma-q4-768-doc-v1")
	if err != nil {
		t.Fatal(err)
	}
	got, err := recommender.Recommend(context.Background(), "user", nil, "calm music", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != "track:b" {
		t.Fatalf("unexpected fused ranking: %v", got)
	}
}

func TestQueryDocumentUsesRetrievalPrompt(t *testing.T) {
	if got := QueryDocument("  спокойная   музыка "); got != "task: search result | query: спокойная музыка" {
		t.Fatalf("unexpected query document %q", got)
	}
}
