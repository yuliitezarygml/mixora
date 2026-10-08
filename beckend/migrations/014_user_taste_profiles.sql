-- Cold-start interests are account state, not likes or a mutable library snapshot.
CREATE TABLE user_taste_profiles (
    user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    artists jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(artists) = 'array'),
    genres jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(genres) = 'array'),
    completed boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now()
);
