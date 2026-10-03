-- Current desired preference is deliberately distinct from the append-only
-- listening_events journal. A neutral value is retained so downstream
-- recommenders can remove a prior signal deterministically.
CREATE TABLE user_track_preferences (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_source text NOT NULL CHECK (char_length(track_source) BETWEEN 1 AND 40),
    track_id text NOT NULL CHECK (char_length(track_id) BETWEEN 1 AND 240),
    preference text NOT NULL CHECK (preference IN ('liked', 'disliked', 'neutral')),
    track_snapshot jsonb NOT NULL,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, track_source, track_id),
    CONSTRAINT user_track_preferences_snapshot_consistent CHECK (
        COALESCE(
            jsonb_typeof(track_snapshot) = 'object'
            AND track_snapshot ?& ARRAY['source', 'id', 'title', 'artist']
            AND track_snapshot->>'source' = track_source
            AND track_snapshot->>'id' = track_id,
            false
        )
    )
);
CREATE INDEX user_track_preferences_user_updated_idx
    ON user_track_preferences (user_id, updated_at DESC, track_source, track_id);

-- Carry existing snapshot-library likes/dislikes into the explicit desired
-- state before the client switches over. A malformed legacy entry is skipped
-- rather than making the application migration fail. If the same reference
-- appears in both arrays, dislike wins: it is the safe state because it keeps
-- the track out of recommendations until the listener makes a new choice.
WITH legacy_entries AS (
    SELECT libraries.user_id, 'liked'::text AS preference, 1 AS priority, value
    FROM user_libraries AS libraries
    CROSS JOIN LATERAL jsonb_array_elements(
        CASE WHEN jsonb_typeof(libraries.payload->'likes') = 'array'
            THEN libraries.payload->'likes' ELSE '[]'::jsonb END
    ) AS entry(value)
    UNION ALL
    SELECT libraries.user_id, 'disliked'::text AS preference, 2 AS priority, value
    FROM user_libraries AS libraries
    CROSS JOIN LATERAL jsonb_array_elements(
        CASE WHEN jsonb_typeof(libraries.payload->'dislikes') = 'array'
            THEN libraries.payload->'dislikes' ELSE '[]'::jsonb END
    ) AS entry(value)
), cleaned_entries AS (
    SELECT
        user_id,
        preference,
        priority,
        lower(trim(regexp_replace(COALESCE(value->>'source', ''), '[[:cntrl:]]', '', 'g'))) AS track_source,
        trim(regexp_replace(COALESCE(value->>'id', ''), '[[:cntrl:]]', '', 'g')) AS raw_track_id,
        COALESCE(NULLIF(trim(regexp_replace(COALESCE(value->>'title', ''), '[[:cntrl:]]', '', 'g')), ''), 'Без названия') AS title,
        COALESCE(NULLIF(trim(regexp_replace(COALESCE(value->>'artist', ''), '[[:cntrl:]]', '', 'g')), ''), 'Исполнитель') AS artist
    FROM legacy_entries
    WHERE jsonb_typeof(value) = 'object'
), canonical_entries AS (
    SELECT
        user_id,
        preference,
        priority,
        track_source,
        CASE
            WHEN track_source = 'soundcloud'
                 AND regexp_match(rtrim(raw_track_id, '/'), '(^|:|/)([0-9]+)$') IS NOT NULL
                THEN (regexp_match(rtrim(raw_track_id, '/'), '(^|:|/)([0-9]+)$'))[2]
            ELSE raw_track_id
        END AS track_id,
        title,
        artist
    FROM cleaned_entries
), selected_entries AS (
    SELECT *, row_number() OVER (
        PARTITION BY user_id, track_source, track_id
        ORDER BY priority DESC
    ) AS rank
    FROM canonical_entries
    WHERE track_source ~ '^[a-z0-9][a-z0-9._-]*$'
      AND char_length(track_source) BETWEEN 1 AND 40
      AND char_length(track_id) BETWEEN 1 AND 240
      AND char_length(title) BETWEEN 1 AND 500
      AND char_length(artist) BETWEEN 1 AND 500
)
INSERT INTO user_track_preferences(
    user_id, track_source, track_id, preference, track_snapshot, revision
)
SELECT
    user_id,
    track_source,
    track_id,
    preference,
    jsonb_build_object(
        'source', track_source, 'id', track_id, 'title', title, 'artist', artist
    ),
    1
FROM selected_entries
WHERE rank = 1
ON CONFLICT (user_id, track_source, track_id) DO NOTHING;

-- The result snapshot is retained per idempotency key so a replay returns the
-- original revision even after a later request changes the current preference.
CREATE TABLE user_track_preference_idempotency (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    idempotency_key text NOT NULL CHECK (char_length(idempotency_key) BETWEEN 1 AND 120),
    request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
    track_source text NOT NULL CHECK (char_length(track_source) BETWEEN 1 AND 40),
    track_id text NOT NULL CHECK (char_length(track_id) BETWEEN 1 AND 240),
    preference text NOT NULL CHECK (preference IN ('liked', 'disliked', 'neutral')),
    track_snapshot jsonb NOT NULL,
    revision bigint NOT NULL CHECK (revision > 0),
    preference_updated_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, idempotency_key),
    FOREIGN KEY (user_id, track_source, track_id)
        REFERENCES user_track_preferences(user_id, track_source, track_id)
        ON DELETE CASCADE,
    CONSTRAINT user_track_preference_idempotency_snapshot_consistent CHECK (
        COALESCE(
            jsonb_typeof(track_snapshot) = 'object'
            AND track_snapshot ?& ARRAY['source', 'id', 'title', 'artist']
            AND track_snapshot->>'source' = track_source
            AND track_snapshot->>'id' = track_id,
            false
        )
    )
);

-- One row per preference key intentionally coalesces pending publication. Each
-- successful desired-state change atomically replaces it with the newest
-- snapshot and revision, so a worker never needs to reconstruct state from a
-- stale event sequence.
CREATE TABLE user_track_preference_outbox (
    user_id uuid NOT NULL,
    track_source text NOT NULL CHECK (char_length(track_source) BETWEEN 1 AND 40),
    track_id text NOT NULL CHECK (char_length(track_id) BETWEEN 1 AND 240),
    desired_preference text NOT NULL CHECK (desired_preference IN ('liked', 'disliked', 'neutral')),
    track_snapshot jsonb NOT NULL,
    revision bigint NOT NULL CHECK (revision > 0),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at timestamptz NOT NULL DEFAULT now(),
    claimed_until timestamptz,
    delivered_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, track_source, track_id),
    FOREIGN KEY (user_id, track_source, track_id)
        REFERENCES user_track_preferences(user_id, track_source, track_id)
        ON DELETE CASCADE,
    CONSTRAINT user_track_preference_outbox_snapshot_consistent CHECK (
        COALESCE(
            jsonb_typeof(track_snapshot) = 'object'
            AND track_snapshot ?& ARRAY['source', 'id', 'title', 'artist']
            AND track_snapshot->>'source' = track_source
            AND track_snapshot->>'id' = track_id,
            false
        )
    )
);
CREATE INDEX user_track_preference_outbox_pending_idx
    ON user_track_preference_outbox (available_at, user_id, track_source, track_id)
    WHERE delivered_at IS NULL;

-- All backfilled and pre-existing desired states receive exactly one durable
-- publication task. A later user write coalesces this row to a newer revision.
INSERT INTO user_track_preference_outbox(
    user_id, track_source, track_id, desired_preference, track_snapshot, revision
)
SELECT user_id, track_source, track_id, preference, track_snapshot, revision
FROM user_track_preferences
ON CONFLICT (user_id, track_source, track_id) DO NOTHING;
