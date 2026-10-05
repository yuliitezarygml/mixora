package gorse

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/iulian/soundcloud-go/internal/library"
	"github.com/iulian/soundcloud-go/internal/music"
)

// PreferencePublisher adapts Mixora's current desired state to Gorse
// feedback. It only publishes tracks that were independently observed in the
// server-side catalog: browser-supplied preference metadata must never create
// a Gorse item or overwrite provider metadata.
type PreferencePublisher struct {
	client   *Client
	verifier TrackVerifier
}

// TrackVerifier is intentionally tiny so Gorse publishers and projectors stay
// independent of the recommendation implementation. PostgresCatalog satisfies
// it directly, and a future trusted provider resolver can satisfy it too.
type TrackVerifier interface {
	Find(context.Context, []string) ([]music.Track, error)
}

func NewPreferencePublisher(client *Client, verifier TrackVerifier) *PreferencePublisher {
	return &PreferencePublisher{client: client, verifier: verifier}
}

func (p *PreferencePublisher) PublishTrackPreference(ctx context.Context, publication library.TrackPreferencePublication) error {
	if p == nil || p.client == nil || p.verifier == nil {
		return errors.New("gorse preference publisher is unavailable")
	}
	userID := strings.TrimSpace(publication.UserID)
	requested := music.CanonicalTrack(music.Track{
		Source: publication.Preference.Track.Source,
		ID:     publication.Preference.Track.ID,
	})
	itemID := requested.Key()
	if userID == "" || requested.Source == "" || requested.ID == "" {
		return errors.New("track preference publication has an incomplete identity")
	}
	preference := publication.Preference.Preference
	switch preference {
	case library.PreferenceLiked, library.PreferenceDisliked, library.PreferenceNeutral:
	default:
		return fmt.Errorf("unsupported track preference %q", preference)
	}
	verified, err := p.verifier.Find(ctx, []string{itemID})
	if err != nil {
		return fmt.Errorf("verify preference track against catalog: %w", err)
	}
	var track music.Track
	for _, candidate := range verified {
		candidate = music.CanonicalTrack(candidate)
		if candidate.Key() == itemID {
			track = candidate
			break
		}
	}
	if track.Source == "" || track.ID == "" {
		if preference == library.PreferenceNeutral {
			// Deletion cannot create a Gorse item, but it can remove feedback from a
			// legacy export that pre-dated catalog verification.
			return p.clearPreferenceFeedback(ctx, userID, itemID)
		}
		// Preserve the user's local desired state, but do not let an unknown
		// client snapshot create an item through a Gorse feedback call. The
		// outbox records a terminal catalog-unverified state and automatically
		// requeues it if the music engine later observes this identity.
		return fmt.Errorf("%w: %s", library.ErrPreferenceTrackUnverified, itemID)
	}
	if err := p.client.UpsertItems(ctx, []music.Track{track}); err != nil {
		return fmt.Errorf("upsert verified preference item: %w", err)
	}
	if err := p.client.UpsertUser(ctx, userID); err != nil {
		return fmt.Errorf("upsert preference user: %w", err)
	}
	timestamp := publication.Preference.UpdatedAt.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	switch preference {
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
		return p.clearPreferenceFeedback(ctx, userID, itemID)
	default:
		return fmt.Errorf("unsupported track preference %q", preference)
	}
}

// clearPreferenceFeedback is safe for a catalog-missing neutral state: Gorse
// DELETE does not create an item, while a previous pre-guard export may still
// need to be removed.
func (p *PreferencePublisher) clearPreferenceFeedback(ctx context.Context, userID, itemID string) error {
	if err := p.client.DeleteFeedback(ctx, "like", userID, itemID); err != nil {
		return fmt.Errorf("remove like feedback: %w", err)
	}
	if err := p.client.DeleteFeedback(ctx, "dislike", userID, itemID); err != nil {
		return fmt.Errorf("remove dislike feedback: %w", err)
	}
	return nil
}

var _ library.TrackPreferencePublisher = (*PreferencePublisher)(nil)
