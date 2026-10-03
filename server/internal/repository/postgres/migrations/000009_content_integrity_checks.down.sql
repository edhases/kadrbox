-- 000009_content_integrity_checks.down.sql
-- Removes the range constraints. The repaired values are NOT restored to their
-- original (invalid) form: negatives, ratings outside 0..10 and the pre-existing
-- mixed-case media_type values are what the constraints were hiding.
ALTER TABLE watch_history DROP CONSTRAINT IF EXISTS chk_watch_history_position_ms;
ALTER TABLE watch_history DROP CONSTRAINT IF EXISTS chk_watch_history_duration_ms;
ALTER TABLE watch_history DROP CONSTRAINT IF EXISTS chk_watch_history_progress_ms;
ALTER TABLE watch_history DROP CONSTRAINT IF EXISTS chk_watch_history_rating;
ALTER TABLE watch_history DROP CONSTRAINT IF EXISTS chk_watch_history_year;
ALTER TABLE watch_history DROP CONSTRAINT IF EXISTS chk_watch_history_season;
ALTER TABLE watch_history DROP CONSTRAINT IF EXISTS chk_watch_history_episode;
ALTER TABLE watch_history DROP CONSTRAINT IF EXISTS chk_watch_history_media_type;
ALTER TABLE favorites    DROP CONSTRAINT IF EXISTS chk_favorites_rating;
ALTER TABLE favorites    DROP CONSTRAINT IF EXISTS chk_favorites_year;
ALTER TABLE favorites    DROP CONSTRAINT IF EXISTS chk_favorites_media_type;