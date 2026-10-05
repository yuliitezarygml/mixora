-- Wave reads the most recently observed non-blocked catalog snapshots as a
-- provider-neutral cold-start fallback. Keep that bounded lookup indexed
-- without indexing blocked Spotify Connect-only metadata.
CREATE INDEX track_catalog_wave_candidates_idx
    ON track_catalog (updated_at DESC, track_source, track_id)
    WHERE COALESCE(payload->>'access', '') <> 'blocked';
