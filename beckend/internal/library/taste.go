package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type TasteProfile struct {
	Artists   []string `json:"artists"`
	Genres    []string `json:"genres"`
	Completed bool     `json:"completed"`
}
type TasteInput struct {
	TasteProfile
	Skip bool `json:"skip"`
}

func NormalizeTaste(input TasteInput) (TasteProfile, error) {
	profile := TasteProfile{Artists: []string{}, Genres: []string{}, Completed: true}
	if input.Skip {
		return profile, nil
	}
	if len(input.Artists) > 30 || len(input.Genres) > 12 {
		return profile, fmt.Errorf("too many interests")
	}
	seen := map[string]bool{}
	for _, artist := range input.Artists {
		artist = strings.TrimSpace(artist)
		if artist == "" || utf8.RuneCountInString(artist) > 120 || strings.ContainsFunc(artist, unicode.IsControl) {
			return profile, fmt.Errorf("invalid artist")
		}
		key := strings.ToLower(artist)
		if !seen[key] {
			profile.Artists = append(profile.Artists, artist)
			seen[key] = true
		}
	}
	if len(profile.Artists) < 5 {
		return profile, fmt.Errorf("choose at least five artists")
	}
	allowed := map[string]bool{"pop": true, "hip-hop": true, "rock": true, "electronic": true, "indie": true, "jazz": true, "classical": true, "ambient": true, "metal": true, "rnb": true, "folk": true, "dance": true}
	seen = map[string]bool{}
	for _, genre := range input.Genres {
		genre = strings.ToLower(strings.TrimSpace(genre))
		if !allowed[genre] {
			return profile, fmt.Errorf("invalid genre")
		}
		if !seen[genre] {
			profile.Genres = append(profile.Genres, genre)
			seen[genre] = true
		}
	}
	return profile, nil
}

func (s *Store) GetTaste(ctx context.Context, userID string) (TasteProfile, error) {
	profile := TasteProfile{Artists: []string{}, Genres: []string{}}
	var artists, genres []byte
	err := s.db.QueryRow(ctx, `SELECT artists, genres, completed FROM user_taste_profiles WHERE user_id=$1`, userID).Scan(&artists, &genres, &profile.Completed)
	if errors.Is(err, pgx.ErrNoRows) {
		return profile, nil
	}
	if err != nil {
		return profile, err
	}
	if err = json.Unmarshal(artists, &profile.Artists); err != nil {
		return profile, err
	}
	err = json.Unmarshal(genres, &profile.Genres)
	return profile, err
}

func (s *Store) PutTaste(ctx context.Context, userID string, input TasteInput) (TasteProfile, error) {
	profile, err := NormalizeTaste(input)
	if err != nil {
		return profile, err
	}
	artists, _ := json.Marshal(profile.Artists)
	genres, _ := json.Marshal(profile.Genres)
	_, err = s.db.Exec(ctx, `INSERT INTO user_taste_profiles(user_id,artists,genres,completed) VALUES($1,$2,$3,true)
        ON CONFLICT(user_id) DO UPDATE SET artists=EXCLUDED.artists,genres=EXCLUDED.genres,completed=true,updated_at=now()`, userID, artists, genres)
	return profile, err
}

func (s *Store) RecommendationTaste(ctx context.Context, userID string) ([]string, []string, error) {
	profile, err := s.GetTaste(ctx, userID)
	return profile.Artists, profile.Genres, err
}
