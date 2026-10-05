-- Reverts media_id from TEXT back to VARCHAR(255).
--
-- NOT REVERSIBLE IN PRACTICE. Rows written while the column was TEXT may hold
-- URLs longer than 255 characters, and narrowing would fail with
-- "value too long for type character varying(255)". Those rows have to be
-- deleted or remapped to a short stable id first, which requires an application
-- change; this file exists so the migration set is complete and the direction
-- is documented, not so it can be run unattended.
--
-- 000011 deliberately widens the column. Use this only on a fresh database or
-- after pruning the over-long rows.
ALTER TABLE watch_party_rooms
    DROP CONSTRAINT IF EXISTS chk_watch_party_rooms_media_id_len;

ALTER TABLE watch_history
    DROP CONSTRAINT IF EXISTS chk_watch_history_media_id_len;

ALTER TABLE favorites
    DROP CONSTRAINT IF EXISTS chk_favorites_media_id_len;

ALTER TABLE watch_party_rooms
    ALTER COLUMN media_id TYPE VARCHAR(255);

ALTER TABLE watch_history
    ALTER COLUMN media_id TYPE VARCHAR(255);

ALTER TABLE favorites
    ALTER COLUMN media_id TYPE VARCHAR(255);
