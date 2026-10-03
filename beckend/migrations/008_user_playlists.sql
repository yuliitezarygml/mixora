-- Account playlists are normalized state. They intentionally do not reuse the
-- source-facing music-engine playlist routes: Mixora owns their names,
-- membership and order, while the engine remains the provider of playable
-- tracks.
CREATE TABLE user_playlists (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    legacy_id text,
    name text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 120),
    description text NOT NULL DEFAULT '' CHECK (char_length(description) <= 2000),
    pinned boolean NOT NULL DEFAULT false,
    liked boolean NOT NULL DEFAULT false,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX user_playlists_legacy_id_unique
    ON user_playlists (user_id, legacy_id)
    WHERE legacy_id IS NOT NULL;
CREATE INDEX user_playlists_user_updated_idx
    ON user_playlists (user_id, updated_at DESC, id);

CREATE TABLE user_playlist_tracks (
    playlist_id uuid NOT NULL REFERENCES user_playlists(id) ON DELETE CASCADE,
    track_source text NOT NULL CHECK (char_length(track_source) BETWEEN 1 AND 40),
    track_id text NOT NULL CHECK (char_length(track_id) BETWEEN 1 AND 240),
    track_snapshot jsonb NOT NULL,
    position integer NOT NULL CHECK (position > 0),
    added_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (playlist_id, track_source, track_id),
    UNIQUE (playlist_id, position),
    CONSTRAINT user_playlist_tracks_snapshot_consistent CHECK (
        COALESCE(
            jsonb_typeof(track_snapshot) = 'object'
            AND track_snapshot ?& ARRAY['source', 'id', 'title', 'artist']
            AND track_snapshot->>'source' = track_source
            AND track_snapshot->>'id' = track_id,
            false
        )
    )
);
CREATE INDEX user_playlist_tracks_order_idx
    ON user_playlist_tracks (playlist_id, position);

-- The old library document was the only home for locally-created playlists.
-- Import valid entries once while retaining a legacy key for traceability; a
-- malformed item is skipped instead of blocking the whole deployment.
WITH legacy_rows AS (
    SELECT libraries.user_id, libraries.payload, libraries.updated_at,
           entry.value, entry.ordinality
    FROM user_libraries AS libraries
    CROSS JOIN LATERAL jsonb_array_elements(
        CASE WHEN jsonb_typeof(libraries.payload->'playlists') = 'array'
            THEN libraries.payload->'playlists' ELSE '[]'::jsonb END
    ) WITH ORDINALITY AS entry(value, ordinality)
), cleaned AS (
    SELECT
        user_id,
        payload,
        updated_at,
        value,
        ordinality,
        CASE
            WHEN char_length(trim(regexp_replace(COALESCE(value->>'id', ''), '[[:cntrl:]]', '', 'g')))
                    BETWEEN 1 AND 120
                THEN trim(regexp_replace(value->>'id', '[[:cntrl:]]', '', 'g'))
            ELSE 'legacy-snapshot-' || ordinality
        END AS legacy_id,
        COALESCE(
            NULLIF(left(trim(regexp_replace(COALESCE(value->>'name', ''), '[[:cntrl:]]', '', 'g')), 120), ''),
            'Плейлист'
        ) AS name,
        left(trim(regexp_replace(COALESCE(value->>'description', ''), '[[:cntrl:]]', '', 'g')), 2000) AS description,
        lower(COALESCE(value->>'liked', '')) IN ('true', '1') AS liked
    FROM legacy_rows
    WHERE jsonb_typeof(value) = 'object'
), selected AS (
    SELECT *, row_number() OVER (
        PARTITION BY user_id, legacy_id
        ORDER BY ordinality ASC
    ) AS duplicate_rank
    FROM cleaned
)
INSERT INTO user_playlists(
    user_id, legacy_id, name, description, pinned, liked, created_at, updated_at
)
SELECT
    user_id,
    legacy_id,
    name,
    description,
    COALESCE(payload->'pins', '[]'::jsonb) @> jsonb_build_array(to_jsonb(legacy_id)),
    liked,
    updated_at,
    updated_at
FROM selected
WHERE duplicate_rank = 1
ON CONFLICT (user_id, legacy_id) WHERE legacy_id IS NOT NULL DO NOTHING;

-- Import ordered, unique track entries after the playlist rows exist. The
-- migration keeps a deliberately compact renderable snapshot; future source
-- reads remain free to refresh richer metadata.
WITH legacy_rows AS (
    SELECT libraries.user_id, entry.value AS playlist_value, entry.ordinality
    FROM user_libraries AS libraries
    CROSS JOIN LATERAL jsonb_array_elements(
        CASE WHEN jsonb_typeof(libraries.payload->'playlists') = 'array'
            THEN libraries.payload->'playlists' ELSE '[]'::jsonb END
    ) WITH ORDINALITY AS entry(value, ordinality)
), playlist_rows AS (
    SELECT
        user_id,
        playlist_value,
        ordinality,
        CASE
            WHEN char_length(trim(regexp_replace(COALESCE(playlist_value->>'id', ''), '[[:cntrl:]]', '', 'g')))
                    BETWEEN 1 AND 120
                THEN trim(regexp_replace(playlist_value->>'id', '[[:cntrl:]]', '', 'g'))
            ELSE 'legacy-snapshot-' || ordinality
        END AS legacy_id
    FROM legacy_rows
    WHERE jsonb_typeof(playlist_value) = 'object'
), raw_tracks AS (
    SELECT playlists.user_id, playlists.legacy_id, track.value, track.ordinality
    FROM playlist_rows AS playlists
    CROSS JOIN LATERAL jsonb_array_elements(
        CASE WHEN jsonb_typeof(playlists.playlist_value->'tracks') = 'array'
            THEN playlists.playlist_value->'tracks' ELSE '[]'::jsonb END
    ) WITH ORDINALITY AS track(value, ordinality)
), cleaned_tracks AS (
    SELECT
        playlists.id AS playlist_id,
        tracks.ordinality,
        lower(trim(regexp_replace(COALESCE(tracks.value->>'source', ''), '[[:cntrl:]]', '', 'g'))) AS track_source,
        trim(regexp_replace(COALESCE(tracks.value->>'id', ''), '[[:cntrl:]]', '', 'g')) AS raw_track_id,
        COALESCE(NULLIF(trim(regexp_replace(COALESCE(tracks.value->>'title', ''), '[[:cntrl:]]', '', 'g')), ''), 'Без названия') AS title,
        COALESCE(NULLIF(trim(regexp_replace(COALESCE(tracks.value->>'artist', ''), '[[:cntrl:]]', '', 'g')), ''), 'Исполнитель') AS artist,
        NULLIF(left(trim(regexp_replace(COALESCE(tracks.value->>'artwork', ''), '[[:cntrl:]]', '', 'g')), 2048), '') AS artwork,
        CASE WHEN lower(COALESCE(tracks.value->>'explicit', '')) IN ('true', '1') THEN true ELSE false END AS explicit,
        NULLIF(left(lower(trim(regexp_replace(COALESCE(tracks.value->>'access', ''), '[[:cntrl:]]', '', 'g'))), 40), '') AS access,
        NULLIF(left(trim(regexp_replace(COALESCE(tracks.value->>'permalink', ''), '[[:cntrl:]]', '', 'g')), 2048), '') AS permalink
    FROM raw_tracks AS tracks
    JOIN user_playlists AS playlists
      ON playlists.user_id = tracks.user_id
     AND playlists.legacy_id = tracks.legacy_id
    WHERE jsonb_typeof(tracks.value) = 'object'
), canonical_tracks AS (
    SELECT
        playlist_id,
        ordinality,
        track_source,
        CASE
            WHEN track_source = 'soundcloud'
                 AND regexp_match(rtrim(raw_track_id, '/'), '(^|:|/)([0-9]+)$') IS NOT NULL
                THEN (regexp_match(rtrim(raw_track_id, '/'), '(^|:|/)([0-9]+)$'))[2]
            ELSE raw_track_id
        END AS track_id,
        title, artist, artwork, explicit, access, permalink
    FROM cleaned_tracks
), deduplicated AS (
    SELECT *, row_number() OVER (
        PARTITION BY playlist_id, track_source, track_id
        ORDER BY ordinality ASC
    ) AS duplicate_rank
    FROM canonical_tracks
    WHERE track_source ~ '^[a-z0-9][a-z0-9._-]*$'
      AND char_length(track_source) BETWEEN 1 AND 40
      AND char_length(track_id) BETWEEN 1 AND 240
      AND char_length(title) BETWEEN 1 AND 500
      AND char_length(artist) BETWEEN 1 AND 500
), ordered AS (
    SELECT *, row_number() OVER (
        PARTITION BY playlist_id
        ORDER BY ordinality ASC
    ) AS position
    FROM deduplicated
    WHERE duplicate_rank = 1
)
INSERT INTO user_playlist_tracks(
    playlist_id, track_source, track_id, track_snapshot, position
)
SELECT
    playlist_id,
    track_source,
    track_id,
    jsonb_strip_nulls(jsonb_build_object(
        'source', track_source,
        'id', track_id,
        'title', title,
        'artist', artist,
        'artwork', artwork,
        'explicit', explicit,
        'access', access,
        'permalink', permalink
    )),
    position
FROM ordered
ON CONFLICT (playlist_id, track_source, track_id) DO NOTHING;

-- Receipts make browser retries and offline queue delivery safe. They do not
-- reference a playlist because successful delete receipts must outlive the
-- playlist row they acknowledge.
CREATE TABLE user_playlist_idempotency (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    idempotency_key text NOT NULL CHECK (char_length(idempotency_key) BETWEEN 1 AND 120),
    request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
    operation text NOT NULL CHECK (operation IN ('replace', 'delete')),
    playlist_id uuid NOT NULL,
    result jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, idempotency_key)
);
