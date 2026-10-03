-- A Wave session represents one delivery of one track to one listener. These
-- receipts make the feedback signal idempotent even if a client submits fresh
-- idempotency keys for the same action, while preserving the general event log
-- for non-Wave activity.
CREATE TABLE wave_event_receipts (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id text NOT NULL,
    event_type text NOT NULL CHECK (event_type IN (
        'play', 'listen_30s', 'complete', 'skip', 'repeat', 'seek',
        'like', 'dislike', 'add_to_playlist'
    )),
    track_source text NOT NULL,
    track_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, session_id, event_type, track_source, track_id)
);
