package models

// User represents a SoundCloud user profile.
type User struct {
	ID              int64   `json:"id"`
	Kind            string  `json:"kind"`
	Permalink       string  `json:"permalink"`
	Username        string  `json:"username"`
	URI             string  `json:"uri"`
	PermalinkURL    string  `json:"permalink_url"`
	AvatarURL       string  `json:"avatar_url"`
	Country         string  `json:"country,omitempty"`
	City            string  `json:"city,omitempty"`
	Description     string  `json:"description,omitempty"`
	FirstName       string  `json:"first_name,omitempty"`
	LastName        string  `json:"last_name,omitempty"`
	FullName        string  `json:"full_name,omitempty"`
	TrackCount      int     `json:"track_count"`
	PlaylistCount   int     `json:"playlist_count"`
	FollowersCount  int     `json:"followers_count"`
	FollowingsCount int     `json:"followings_count"`
	LikesCount      int     `json:"likes_count"`
	CommentsCount   int     `json:"comments_count"`
	Verified        bool     `json:"verified"`
	Badges          *Badges  `json:"badges,omitempty"`
	Visuals         *Visuals `json:"visuals,omitempty"`
}

// Visuals holds the user's header/banner images.
type Visuals struct {
	URN     string   `json:"urn"`
	Enabled bool     `json:"enabled"`
	Visuals []Visual `json:"visuals"`
}

// Visual represents a single visual banner image.
type Visual struct {
	URN       string `json:"urn"`
	EntryTime int    `json:"entry_time"`
	VisualURL string `json:"visual_url"`
}

// BannerURL returns the user's header banner image URL if present.
func (u *User) BannerURL() string {
	if u.Visuals != nil && len(u.Visuals.Visuals) > 0 {
		return u.Visuals.Visuals[0].VisualURL
	}
	return ""
}

// AvatarURLSize returns the avatar URL resized to the requested size.
// Supported sizes: "t500x500", "t300x300", "crop", "original", "large", "badge", "small", "tiny", "mini".
func (u *User) AvatarURLSize(size string) string {
	if u.AvatarURL == "" {
		return ""
	}
	return replaceImageSize(u.AvatarURL, size)
}

// WebProfile represents a third-party social link for a user.
type WebProfile struct {
	Kind    string `json:"kind"`
	ID      int64  `json:"id"`
	Service string `json:"service"`
	Title   string `json:"title"`
	URL     string `json:"url"`
}
