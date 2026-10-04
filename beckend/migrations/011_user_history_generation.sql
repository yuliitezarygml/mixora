-- A clear must win over an earlier offline/in-flight listen that reaches the
-- API later. The monotonic per-account generation gives those operations a
-- durable fence without changing the already-applied history migration.
CREATE TABLE user_history_state (
    user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    generation bigint NOT NULL DEFAULT 0 CHECK (generation >= 0),
    updated_at timestamptz NOT NULL DEFAULT now()
);
