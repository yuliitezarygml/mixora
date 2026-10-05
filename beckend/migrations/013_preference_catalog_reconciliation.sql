-- Positive preferences received before their provider track is observed must
-- never create a Gorse item. Keep a durable marker so the outbox can requeue
-- exactly those terminal-skipped rows after track_catalog gains the identity.
ALTER TABLE user_track_preference_outbox
    ADD COLUMN catalog_unverified_at timestamptz;

CREATE INDEX user_track_preference_outbox_catalog_unverified_idx
    ON user_track_preference_outbox (catalog_unverified_at, track_source, track_id)
    WHERE catalog_unverified_at IS NOT NULL;
