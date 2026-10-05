-- 000005_remove_hdrezka.up.sql
--
-- ONE-TIME DATA MIGRATION. It deletes rows and it is only safe to run once.
--
-- Before schema_migrations existed (added by the migration runner alongside
-- migration 000006) the embedded runner re-executed *every* file on *every*
-- process start, so these three DELETEs ran forever: a not-yet-updated client
-- that re-synced its local hdrezka rows had them silently destroyed again on the
-- next restart, with no record that it ever happened. The version ledger is what
-- makes the script one-shot; from now on the runner applies it exactly once and
-- records "000005_remove_hdrezka".
--
-- Why deleting is safe:
--   * The provider was removed from the server-side registry, so the server can
--     no longer validate, refresh or serve these entries.
--   * The client's own store is the source of truth for its favourites and
--     watch progress; a user who still has the provider can resync locally.
--   * content_cache holds only re-fetchable parsed metadata with a TTL.
--
-- NOTE FOR THE FIRST DEPLOY OF THIS VERSION: existing installations re-run this
-- file once under the new runner, because schema_migrations starts empty. That is
-- the intended one-time pass, but it means any hdrezka row that a stale client
-- pushed since the previous boot is deleted at that moment. Deploy it together
-- with the sync-handler change that stops emitting hdrezka rows.
DELETE FROM favorites      WHERE provider_id = 'hdrezka';
DELETE FROM watch_history  WHERE provider_id = 'hdrezka';
DELETE FROM content_cache  WHERE provider_id = 'hdrezka';