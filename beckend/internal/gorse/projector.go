package gorse

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/iulian/soundcloud-go/internal/events"
	"github.com/iulian/soundcloud-go/internal/music"
)

type Projector struct {
	client   *Client
	verifier TrackVerifier
}

func NewProjector(client *Client, verifier TrackVerifier) *Projector {
	return &Projector{client: client, verifier: verifier}
}

func (p *Projector) Project(ctx context.Context, aggregates []events.FeedbackAggregate) error {
	if p == nil || p.client == nil || p.verifier == nil {
		return errors.New("gorse projector is unavailable")
	}
	type candidate struct {
		aggregate events.FeedbackAggregate
		itemID    string
	}
	candidates := make([]candidate, 0, len(aggregates))
	keys := make([]string, 0, len(aggregates))
	seenKeys := make(map[string]struct{}, len(aggregates))
	for _, aggregate := range aggregates {
		userID := strings.TrimSpace(aggregate.UserID)
		track := music.CanonicalTrack(music.Track{Source: aggregate.Source, ID: aggregate.TrackID})
		if userID == "" || track.Source == "" || track.ID == "" {
			continue
		}
		itemID := track.Key()
		aggregate.UserID = userID
		aggregate.Source = track.Source
		aggregate.TrackID = track.ID
		candidates = append(candidates, candidate{aggregate: aggregate, itemID: itemID})
		if _, exists := seenKeys[itemID]; !exists {
			seenKeys[itemID] = struct{}{}
			keys = append(keys, itemID)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	verified, err := p.verifier.Find(ctx, keys)
	if err != nil {
		return fmt.Errorf("verify event tracks against catalog: %w", err)
	}
	byKey := make(map[string]music.Track, len(verified))
	for _, track := range verified {
		track = music.CanonicalTrack(track)
		if track.Source != "" && track.ID != "" {
			byKey[track.Key()] = track
		}
	}

	// A normal listening event is durable product data even if its source has
	// not been independently observed yet. Keep it in PostgreSQL, but terminal-
	// skip its Gorse projection so browser-supplied metadata cannot manufacture
	// an item. A verifier outage returns an error above and is retried by the
	// projection worker instead.
	filtered := make([]candidate, 0, len(candidates))
	items := make([]music.Track, 0, len(keys))
	for _, key := range keys {
		if track, exists := byKey[key]; exists {
			items = append(items, track)
		}
	}
	for _, candidate := range candidates {
		if _, exists := byKey[candidate.itemID]; exists {
			filtered = append(filtered, candidate)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	if err := p.client.UpsertItems(ctx, items); err != nil {
		return fmt.Errorf("upsert verified event items: %w", err)
	}
	users := make(map[string]struct{}, len(filtered))
	for _, candidate := range filtered {
		if _, exists := users[candidate.aggregate.UserID]; exists {
			continue
		}
		if err := p.client.UpsertUser(ctx, candidate.aggregate.UserID); err != nil {
			return fmt.Errorf("upsert event user: %w", err)
		}
		users[candidate.aggregate.UserID] = struct{}{}
	}

	feedback := make([]Feedback, 0, len(aggregates))
	for _, candidate := range filtered {
		aggregate := candidate.aggregate
		timestamp := aggregate.LatestAt
		if timestamp.IsZero() {
			timestamp = time.Now().UTC()
		}
		feedback = append(feedback, Feedback{
			FeedbackType: aggregate.Type,
			UserID:       aggregate.UserID,
			ItemID:       candidate.itemID,
			Timestamp:    timestamp.UTC(),
			Value:        aggregate.Count * feedbackWeight(aggregate.Type),
		})
	}
	return p.client.PutFeedback(ctx, feedback)
}

func feedbackWeight(eventType string) float64 {
	switch eventType {
	case "like", "add_to_playlist":
		return 3
	case "repeat", "complete":
		return 2
	case "listen_30s":
		return 1.5
	case "dislike":
		return 3
	case "skip":
		return 1.5
	case "play":
		return 1
	case "impression", "seek":
		return 0.25
	default:
		return 1
	}
}
