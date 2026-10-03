package library

import (
	"context"

	"github.com/iulian/soundcloud-go/internal/music"
)

// RecommendationTracks exposes the complete current desired state in the
// narrow form used by Wave. Neutral records must be returned too: a neutral
// state is a server-owned retraction and has to remove an older optimistic
// like or dislike sent by another device.
func (s *Store) RecommendationTracks(ctx context.Context, userID string) (likes []music.Track, dislikes []music.Track, neutral []music.Track, err error) {
	preferences, err := s.ListTrackPreferences(ctx, userID)
	if err != nil {
		return nil, nil, nil, err
	}
	return partitionRecommendationTracks(preferences)
}

func partitionRecommendationTracks(preferences []TrackPreference) (likes []music.Track, dislikes []music.Track, neutral []music.Track, err error) {
	for _, preference := range preferences {
		switch preference.Preference {
		case PreferenceLiked:
			likes = append(likes, preference.Track.musicTrack())
		case PreferenceDisliked:
			dislikes = append(dislikes, preference.Track.musicTrack())
		case PreferenceNeutral:
			neutral = append(neutral, preference.Track.musicTrack())
		}
	}
	return likes, dislikes, neutral, nil
}
