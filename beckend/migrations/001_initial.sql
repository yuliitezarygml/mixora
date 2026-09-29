CREATE TABLE users (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 email text NOT NULL UNIQUE,
 password_hash text NOT NULL,
 display_name text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 80),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE sessions (
 token_hash text PRIMARY KEY,
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_expiry_idx ON sessions(expires_at);
CREATE TABLE tracks (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 title text NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
 artist text NOT NULL CHECK (length(artist) BETWEEN 1 AND 200),
 album text NOT NULL DEFAULT '',
 explicit boolean NOT NULL DEFAULT false,
 media_key text NOT NULL UNIQUE,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE playlists (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 name text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX playlists_user_idx ON playlists(user_id);
CREATE TABLE playlist_tracks (
 playlist_id uuid NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
 track_id uuid NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
 added_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (playlist_id, track_id)
);
