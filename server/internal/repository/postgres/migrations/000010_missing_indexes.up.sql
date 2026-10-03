-- 000010_missing_indexes.up.sql
--
-- Indexes for predicates that had none, plus a sargable replacement for the
-- "continue watching" one.
--
-- * content_cache.provider_id: migration 000005 filters on it and the cache
--   sweeper/report paths select by provider. It is the one FK-ish column of that
--   table with no index. (cache_repo.go is owned elsewhere; index creation is
--   schema work, so it belongs here.)
-- * watch_party_rooms.host_id: a foreign key column with no index, so listing a
--   user's rooms or cascading a user deletion has to scan the whole table.
-- * idx_history_continue_v2: GetContinueWatching used
--     CAST(position_ms AS FLOAT) / CAST(duration_ms AS FLOAT) BETWEEN 0.05 AND 0.95
--   A per-row division is not sargable — no index on position_ms/duration_ms can
--   be used across it. The rewritten integer predicate (position_ms * 100 vs
--   duration_ms * 5 / * 95) is not range-seekable either, because the bound
--   depends on a second column, so the useful index is a *partial* one that
--   already contains only the rows the query can return and provides the
--   (user_id, watched_at DESC) ordering for free. The old
--   idx_history_continue (user_id, position_ms, duration_ms) cannot serve this
--   query any more and is dropped.
--
-- The old index is dropped in the same transaction that creates the new one so
-- the table is never left with neither.

CREATE INDEX IF NOT EXISTS idx_content_cache_provider ON content_cache(provider_id);
CREATE INDEX IF NOT EXISTS idx_content_cache_provider_expires ON content_cache(provider_id, expires_at);
CREATE INDEX IF NOT EXISTS idx_watch_party_rooms_host ON watch_party_rooms(host_id);

CREATE INDEX IF NOT EXISTS idx_history_continue_v2
    ON watch_history (user_id, watched_at DESC, id DESC)
    WHERE duration_ms > 0 AND position_ms > 0;

DROP INDEX IF EXISTS idx_history_continue;