-- 008_user_playlists.sql has already shipped and its checksum must remain
-- immutable. Its metadata import correctly chose the first duplicate legacy
-- playlist id, but its track import could read tracks from every duplicate.
-- Rebuild only untouched migrated playlists from that same first occurrence.
-- A playlist edited after migration has updated_at different from its legacy
-- library snapshot and is deliberately left alone.
WITH legacy_rows AS (
    SELECT libraries.user_id, libraries.updated_at, entry.value, entry.ordinality
    FROM user_libraries AS libraries
    CROSS JOIN LATERAL jsonb_array_elements(
        CASE WHEN jsonb_typeof(libraries.payload->'playlists') = 'array'
            THEN libraries.payload->'playlists' ELSE '[]'::jsonb END
    ) WITH ORDINALITY AS entry(value, ordinality)
), cleaned AS (
    SELECT
        user_id,
        updated_at,
        ordinality,
        CASE
            WHEN char_length(trim(regexp_replace(COALESCE(value->>'id', ''), '[[:cntrl:]]', '', 'g')))
                    BETWEEN 1 AND 120
                THEN trim(regexp_replace(value->>'id', '[[:cntrl:]]', '', 'g'))
            ELSE 'legacy-snapshot-' || ordinality
        END AS legacy_id
    FROM legacy_rows
    WHERE jsonb_typeof(value) = 'object'
), selected AS (
    SELECT *, row_number() OVER (
        PARTITION BY user_id, legacy_id
        ORDER BY ordinality ASC
    ) AS duplicate_rank
    FROM cleaned
)
DELETE FROM user_playlist_tracks AS tracks
USING user_playlists AS playlists
JOIN user_libraries AS libraries
  ON libraries.user_id = playlists.user_id
JOIN selected
  ON selected.user_id = playlists.user_id
 AND selected.legacy_id = playlists.legacy_id
 AND selected.duplicate_rank = 1
WHERE tracks.playlist_id = playlists.id
  AND playlists.updated_at = libraries.updated_at;

WITH legacy_rows AS (
    SELECT libraries.user_id, libraries.updated_at, entry.value, entry.ordinality
    FROM user_libraries AS libraries
    CROSS JOIN LATERAL jsonb_array_elements(
        CASE WHEN jsonb_typeof(libraries.payload->'playlists') = 'array'
            THEN libraries.payload->'playlists' ELSE '[]'::jsonb END
    ) WITH ORDINALITY AS entry(value, ordinality)
), cleaned AS (
    SELECT
        user_id,
        updated_at,
        value,
        ordinality,
        CASE
            WHEN char_length(trim(regexp_replace(COALESCE(value->>'id', ''), '[[:cntrl:]]', '', 'g')))
                    BETWEEN 1 AND 120
                THEN trim(regexp_replace(value->>'id', '[[:cntrl:]]', '', 'g'))
            ELSE 'legacy-snapshot-' || ordinality
        END AS legacy_id
    FROM legacy_rows
    WHERE jsonb_typeof(value) = 'object'
), selected AS (
    SELECT *, row_number() OVER (
        PARTITION BY user_id, legacy_id
        ORDER BY ordinality ASC
    ) AS duplicate_rank
    FROM cleaned
), raw_tracks AS (
    SELECT selected.user_id, selected.updated_at, selected.legacy_id,
           track.value, track.ordinality
    FROM selected
    CROSS JOIN LATERAL jsonb_array_elements(
        CASE WHEN jsonb_typeof(selected.value->'tracks') = 'array'
            THEN selected.value->'tracks' ELSE '[]'::jsonb END
    ) WITH ORDINALITY AS track(value, ordinality)
    WHERE selected.duplicate_rank = 1
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
    JOIN user_libraries AS libraries
      ON libraries.user_id = playlists.user_id
     AND playlists.updated_at = libraries.updated_at
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
