package gorse

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/iulian/soundcloud-go/internal/library"
)

// PreferencePublisher adapts Mixora's current desired state to Gorse
// feedback. It deliberately publishes opaque track keys only: catalog metadata
// continues to be hydrated from the existing music backend, not from a
// client-provided preference request.
type PreferencePublisher struct {
	client *Client
}

func NewPreferencePublisher(client *Client) *PreferencePublisher {
	return &PreferencePublisher{client: client}
}

func (p *PreferencePublisher) PublishTrackPreference(ctx context.Context, publication library.TrackPreferencePublication) error {
	if p == nil || p.client == nil {
		return errors.New("gorse preference publisher is unavailable")
	}
	userID := strings.TrimSpace(publication.UserID)
	track := publication.Preference.Track
	itemID := strings.TrimSpace(track.Source) + ":" + strings.TrimSpace(track.ID)
	if userID == "" || strings.TrimSpace(track.Source) == "" || strings.TrimSpace(track.ID) == "" {
		return errors.New("track preference publication has an incomplete identity")
	}
	if err := p.client.UpsertUser(ctx, userID); err != nil {
		return fmt.Errorf("upsert preference user: %w", err)
	}
	timestamp := publication.Preference.UpdatedAt.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	switch publication.Preference.Preference {
	case library.PreferenceLiked:
		if err := p.client.DeleteFeedback(ctx, "dislike", userID, itemID); err != nil {
			return fmt.Errorf("remove opposite dislike feedback: %w", err)
		}
		if err := p.client.PutFeedback(ctx, []Feedback{{
			FeedbackType: "like", UserID: userID, ItemID: itemID, Timestamp: timestamp, Value: 1,
		}}); err != nil {
			return fmt.Errorf("publish like feedback: %w", err)
		}
		return nil
	case library.PreferenceDisliked:
		if err := p.client.DeleteFeedback(ctx, "like", userID, itemID); err != nil {
			return fmt.Errorf("remove opposite like feedback: %w", err)
		}
		if err := p.client.PutFeedback(ctx, []Feedback{{
			FeedbackType: "dislike", UserID: userID, ItemID: itemID, Timestamp: timestamp, Value: 1,
		}}); err != nil {
			return fmt.Errorf("publish dislike feedback: %w", err)
		}
		return nil
	case library.PreferenceNeutral:
		if err := p.client.DeleteFeedback(ctx, "like", userID, itemID); err != nil {
			return fmt.Errorf("remove like feedback: %w", err)
		}
		if err := p.client.DeleteFeedback(ctx, "dislike", userID, itemID); err != nil {
			return fmt.Errorf("remove dislike feedback: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported track preference %q", publication.Preference.Preference)
	}
}

var _ library.TrackPreferencePublisher = (*PreferencePublisher)(nil)
