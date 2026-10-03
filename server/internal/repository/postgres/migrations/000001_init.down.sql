-- 000001_init.down.sql
-- WARNING: destructive. Reverses 000001_init by dropping every table it created,
-- which deletes all users, favourites, watch history, cached metadata and watch
-- party rooms. Only meaningful against an empty/test database.
DROP INDEX IF EXISTS idx_active_room_code;
DROP TABLE IF EXISTS watch_party_rooms;
DROP TABLE IF EXISTS content_cache;
DROP TABLE IF EXISTS watch_history;
DROP TABLE IF EXISTS favorites;
DROP TABLE IF EXISTS users;