-- media_id holds the provider's own item URL, not a short hash.
--
-- The client keys favourites and watch history by that URL because it is also
-- the lookup key for /content/details and /content/streams, so it cannot be
-- replaced by a stable hash without breaking those round trips. Items whose
-- identifier is a JSON envelope rather than a bare path routinely exceed the
-- 255 characters VARCHAR allowed: the sync insert then failed with
-- "value too long for type character varying(255)" and surfaced as HTTP 500.
--
-- TEXT is required rather than a wider VARCHAR. The upper bound is enforced by
-- a CHECK instead, chosen to stay inside PostgreSQL's btree index limit
-- (~2704 bytes for the whole key) so the unique constraints remain creatable:
-- user_id (16) + provider_id (100) + season/episode (8) leaves ~2580 bytes.
ALTER TABLE favorites
    ALTER COLUMN media_id TYPE TEXT;

ALTER TABLE watch_history
    ALTER COLUMN media_id TYPE TEXT;

ALTER TABLE watch_party_rooms
    ALTER COLUMN media_id TYPE TEXT;

DO $$
DECLARE
    t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['favorites', 'watch_history', 'watch_party_rooms'] LOOP
        IF NOT EXISTS (
            SELECT 1 FROM pg_constraint
            WHERE conname = 'chk_' || t || '_media_id_len'
              AND connamespace = 'public'::regnamespace
        ) THEN
            EXECUTE format(
                'ALTER TABLE %I ADD CONSTRAINT %I CHECK (char_length(media_id) <= 2000)',
                t, 'chk_' || t || '_media_id_len'
            );
        END IF;
    END LOOP;
END $$;
