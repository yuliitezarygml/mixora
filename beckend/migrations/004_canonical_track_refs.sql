UPDATE listening_events
SET track_id = substring(track_id FROM '([0-9]+)$'),
    recommendation_projected_at = NULL,
    recommendation_projection_error = NULL,
    recommendation_projection_available_at = now()
WHERE lower(track_source) = 'soundcloud'
  AND track_id ~ '(^|:|/)[0-9]+$'
  AND track_id !~ '^[0-9]+$';

UPDATE recommendation_impressions
SET track_id = substring(track_id FROM '([0-9]+)$')
WHERE lower(track_source) = 'soundcloud'
  AND track_id ~ '(^|:|/)[0-9]+$'
  AND track_id !~ '^[0-9]+$';
