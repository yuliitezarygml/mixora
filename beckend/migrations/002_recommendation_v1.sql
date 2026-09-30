CREATE TABLE track_catalog (
    track_source text NOT NULL,
    track_id text NOT NULL,
    payload jsonb NOT NULL,
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (track_source, track_id)
);

ALTER TABLE listening_events
    ADD COLUMN recommendation_projected_at timestamptz,
    ADD COLUMN recommendation_projection_attempts integer NOT NULL DEFAULT 0,
    ADD COLUMN recommendation_projection_error text,
    ADD COLUMN recommendation_projection_available_at timestamptz NOT NULL DEFAULT now();

CREATE INDEX listening_events_recommendation_projection_idx
    ON listening_events (recommendation_projection_available_at, created_at)
    WHERE recommendation_projected_at IS NULL
      AND track_source <> ''
      AND track_id <> '';
