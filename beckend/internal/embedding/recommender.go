package embedding

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

type ContentStore interface {
	TasteSeeds(context.Context, string, int) ([]string, error)
	Similar(context.Context, []string, int) ([]string, error)
	Nearest(context.Context, []float32, []string, int) ([]string, error)
}

type Recommender struct {
	store    ContentStore
	embedder Embedder
	version  string
}

func NewRecommender(store ContentStore, embedder Embedder, version string) (*Recommender, error) {
	if store == nil || embedder == nil {
		return nil, errors.New("content store and embedder are required")
	}
	if strings.TrimSpace(version) == "" {
		return nil, errors.New("embedding version is required")
	}
	return &Recommender{store: store, embedder: embedder, version: version}, nil
}

func (r *Recommender) Version() string { return r.version }

func (r *Recommender) Recommend(
	ctx context.Context,
	userID string,
	fallbackSeeds []string,
	query string,
	limit int,
) ([]string, error) {
	serverSeeds, seedErr := r.store.TasteSeeds(ctx, userID, 50)
	seeds := uniqueKeys(50, serverSeeds, fallbackSeeds)
	taste, tasteErr := r.store.Similar(ctx, seeds, max(limit, 1))

	var queryRanked []string
	var queryErr error
	if strings.TrimSpace(query) != "" {
		queryCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		vectors, err := r.embedder.Embed(queryCtx, []string{QueryDocument(query)})
		if err != nil {
			queryErr = err
		} else if len(vectors) == 1 {
			queryRanked, queryErr = r.store.Nearest(queryCtx, vectors[0], seeds, max(limit, 1))
		}
		cancel()
	}

	result := reciprocalRankFuse([]weightedRanking{
		{keys: taste, weight: 0.65},
		{keys: queryRanked, weight: 0.35},
	}, limit)
	if len(result) > 0 {
		return result, nil
	}
	return nil, errors.Join(seedErr, tasteErr, queryErr)
}

func uniqueKeys(limit int, groups ...[]string) []string {
	result := make([]string, 0, limit)
	seen := make(map[string]bool, limit)
	for _, values := range groups {
		for _, value := range values {
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			result = append(result, value)
			if len(result) == limit {
				return result
			}
		}
	}
	return result
}

type weightedRanking struct {
	keys   []string
	weight float64
}

func reciprocalRankFuse(rankings []weightedRanking, limit int) []string {
	if limit <= 0 {
		return nil
	}
	type score struct {
		key   string
		value float64
		first int
	}
	byKey := make(map[string]*score)
	order := 0
	for _, ranking := range rankings {
		seen := make(map[string]bool, len(ranking.keys))
		for index, key := range ranking.keys {
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			entry := byKey[key]
			if entry == nil {
				entry = &score{key: key, first: order}
				byKey[key] = entry
				order++
			}
			entry.value += ranking.weight / float64(60+index+1)
		}
	}
	scores := make([]score, 0, len(byKey))
	for _, value := range byKey {
		scores = append(scores, *value)
	}
	sort.SliceStable(scores, func(i, j int) bool {
		if scores[i].value == scores[j].value {
			return scores[i].first < scores[j].first
		}
		return scores[i].value > scores[j].value
	})
	result := make([]string, 0, min(limit, len(scores)))
	for _, value := range scores {
		result = append(result, value.key)
		if len(result) == limit {
			break
		}
	}
	return result
}
