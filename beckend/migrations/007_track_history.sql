-- User-facing history is a compact, normalized view over listening activity.
-- The append-only listening_events journal remains the recommendation source;
-- this table makes recent tracks renderable across devices without duplicating
-- the music engine's catalog or stream metadata.
CREATE TABLE user_track_history (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_source text NOT NULL CHECK (char_length(track_source) BETWEEN 1 AND 40),
    track_id text NOT NULL CHECK (char_length(track_id) BETWEEN 1 AND 240),
    track_snapshot jsonb NOT NULL,
    first_listened_at timestamptz NOT NULL,
    last_listened_at timestamptz NOT NULL,
    play_count integer NOT NULL DEFAULT 1 CHECK (play_count > 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, track_source, track_id),
    CONSTRAINT user_track_history_snapshot_consistent CHECK (
        COALESCE(
            jsonb_typeof(track_snapshot) = 'object'
            AND track_snapshot ?& ARRAY['source', 'id', 'title', 'artist']
            AND track_snapshot->>'source' = track_source
            AND track_snapshot->>'id' = track_id,
            false
        )
    )
);
CREATE INDEX user_track_history_recent_idx
    ON user_track_history (user_id, last_listened_at DESC, track_source, track_id);

-- The snapshot library was the pre-normalization history source. Preserve its
-- renderable tracks during migration, using its array order (newest first) as
-- a stable tie breaker when old entries did not retain an occurrence time.
WITH legacy_entries AS (
    SELECT libraries.user_id, libraries.updated_at, entry.value, entry.ordinality
    FROM user_libraries AS libraries
    CROSS JOIN LATERAL jsonb_array_elements(
        CASE WHEN jsonb_typeof(libraries.payload->'history') = 'array'
            THEN libraries.payload->'history' ELSE '[]'::jsonb END
    ) WITH ORDINALITY AS entry(value, ordinality)
), cleaned_entries AS (
    SELECT
        user_id,
        updated_at - (ordinality * interval '1 microsecond') AS listened_at,
        ordinality,
        lower(trim(regexp_replace(COALESCE(value->>'source', ''), '[[:cntrl:]]', '', 'g'))) AS track_source,
        trim(regexp_replace(COALESCE(value->>'id', ''), '[[:cntrl:]]', '', 'g')) AS raw_track_id,
        COALESCE(NULLIF(trim(regexp_replace(COALESCE(value->>'title', ''), '[[:cntrl:]]', '', 'g')), ''), 'Без названия') AS title,
        COALESCE(NULLIF(trim(regexp_replace(COALESCE(value->>'artist', ''), '[[:cntrl:]]', '', 'g')), ''), 'Исполнитель') AS artist
    FROM legacy_entries
    WHERE jsonb_typeof(value) = 'object'
), canonical_entries AS (
    SELECT
        user_id,
        listened_at,
        ordinality,
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
        ORDER BY ordinality ASC
    ) AS rank
    FROM canonical_entries
    WHERE track_source ~ '^[a-z0-9][a-z0-9._-]*$'
      AND char_length(track_source) BETWEEN 1 AND 40
      AND char_length(track_id) BETWEEN 1 AND 240
      AND char_length(title) BETWEEN 1 AND 500
      AND char_length(artist) BETWEEN 1 AND 500
)
INSERT INTO user_track_history(
    user_id, track_source, track_id, track_snapshot,
    first_listened_at, last_listened_at, play_count
)
SELECT
    user_id,
    track_source,
    track_id,
    jsonb_build_object(
        'source', track_source, 'id', track_id, 'title', title, 'artist', artist
    ),
    listened_at,
    listened_at,
    1
FROM selected_entries
WHERE rank = 1
ON CONFLICT (user_id, track_source, track_id) DO NOTHING;

-- A browser may retry an offline history record. Store the original result so
-- the retry is safe and cannot inflate the listening counter.
CREATE TABLE user_track_history_idempotency (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    idempotency_key text NOT NULL CHECK (char_length(idempotency_key) BETWEEN 1 AND 120),
    request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
    track_source text NOT NULL CHECK (char_length(track_source) BETWEEN 1 AND 40),
    track_id text NOT NULL CHECK (char_length(track_id) BETWEEN 1 AND 240),
    track_snapshot jsonb NOT NULL,
    first_listened_at timestamptz NOT NULL,
    last_listened_at timestamptz NOT NULL,
    play_count integer NOT NULL CHECK (play_count > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, idempotency_key),
    FOREIGN KEY (user_id, track_source, track_id)
        REFERENCES user_track_history(user_id, track_source, track_id)
        ON DELETE CASCADE,
    CONSTRAINT user_track_history_idempotency_snapshot_consistent CHECK (
        COALESCE(
            jsonb_typeof(track_snapshot) = 'object'
            AND track_snapshot ?& ARRAY['source', 'id', 'title', 'artist']
            AND track_snapshot->>'source' = track_source
            AND track_snapshot->>'id' = track_id,
            false
        )
    )
);
