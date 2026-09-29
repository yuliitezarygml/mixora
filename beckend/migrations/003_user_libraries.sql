CREATE TABLE user_libraries (
 user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
 data jsonb NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);
