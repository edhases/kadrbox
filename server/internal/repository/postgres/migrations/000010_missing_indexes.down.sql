-- 000010_missing_indexes.down.sql
-- Restores idx_history_continue and removes the new indexes. Note that the
-- restored index is not usable by the rewritten integer predicate, so reverting
-- this migration also means reverting the GetContinueWatching rewrite to keep the
-- query plan coherent.
CREATE INDEX IF NOT EXISTS idx_history_continue ON watch_history(user_id, position_ms, duration_ms);
DROP INDEX IF EXISTS idx_history_continue_v2;
DROP INDEX IF EXISTS idx_content_cache_provider_expires;
DROP INDEX IF EXISTS idx_content_cache_provider;
DROP INDEX IF EXISTS idx_watch_party_rooms_host;