package library

import "testing"

func TestTasteRequiresFiveDistinctArtistsAndDoesNotInventLikes(t *testing.T) {
	input := TasteInput{TasteProfile: TasteProfile{Artists: []string{"Tycho", "RAC", "Miyagi", "ODESZA", "Daft Punk"}, Genres: []string{"electronic", "electronic"}}}
	result, err := NormalizeTaste(input)
	if err != nil || !result.Completed || len(result.Artists) != 5 || len(result.Genres) != 1 {
		t.Fatalf("profile=%#v error=%v", result, err)
	}
	for _, artists := range [][]string{{"One"}, {"One", "one", "Two", "Three", "Four"}} {
		if _, err := NormalizeTaste(TasteInput{TasteProfile: TasteProfile{Artists: artists}}); err == nil {
			t.Fatal("invalid count accepted")
		}
	}
	input.Genres = []string{"unknown"}
	if _, err := NormalizeTaste(input); err == nil {
		t.Fatal("invalid genre accepted")
	}
	skipped, err := NormalizeTaste(TasteInput{Skip: true})
	if err != nil || !skipped.Completed || len(skipped.Artists) != 0 {
		t.Fatal("explicit skip was not persisted")
	}
}
