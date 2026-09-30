package gorse

import (
	"context"
	"strings"
	"time"

	"github.com/iulian/soundcloud-go/internal/events"
)

type Projector struct {
	client *Client
}

func NewProjector(client *Client) *Projector {
	return &Projector{client: client}
}

func (p *Projector) Project(ctx context.Context, aggregates []events.FeedbackAggregate) error {
	feedback := make([]Feedback, 0, len(aggregates))
	for _, aggregate := range aggregates {
		if strings.TrimSpace(aggregate.UserID) == "" || strings.TrimSpace(aggregate.Source) == "" || strings.TrimSpace(aggregate.TrackID) == "" {
			continue
		}
		timestamp := aggregate.LatestAt
		if timestamp.IsZero() {
			timestamp = time.Now().UTC()
		}
		feedback = append(feedback, Feedback{
			FeedbackType: aggregate.Type,
			UserID:       aggregate.UserID,
			ItemID:       aggregate.Source + ":" + aggregate.TrackID,
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
