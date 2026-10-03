-- 000009_content_integrity_checks.up.sql
--
-- Value-range constraints for the columns the clients write into.
--
-- Nothing here rejects a shape the application legitimately produces. The bounds
-- were chosen from the code, not invented:
--   * rating is an IMDb/kp-derived 0..10 score scraped with a loose regex, so
--     out-of-range values are real garbage from providers, not valid input.
--   * year: 0 is this codebase's "unknown year" sentinel (ParseFlexibleYear
--     returns 0 for unparseable input), so 0 must stay allowed.
--   * season/episode: >= 1. Episode 0 is a "special" marker upstream but not a
--     row the client can address; NULL is the honest representation.
--   * media_type is a lowercase slug; the repository now normalises it on write.
--
-- The repository (history_repo.go / favorites_repo.go) sanitises every one of
-- these values before the statement, so these constraints are a backstop that
-- cannot turn a malformed client payload into a failed sync request.
--
-- Existing data is repaired rather than blocked on, and the repairs are
-- value-preserving: numeric values are clamped into range rather than nulled, so
-- no score or year is lost. Only values that are not representable at all
-- (a non-slug media_type) are dropped to NULL, and season/episode 0 becomes NULL
-- (its documented meaning).
--
-- Constraint creation goes through a loop because ALTER TABLE ... ADD CONSTRAINT
-- has no IF NOT EXISTS form; the pg_constraint probe is what makes this migration
-- re-runnable after a partially applied attempt.

-- --- repairs -----------------------------------------------------------------
-- NaN is a legal REAL in PostgreSQL and compares greater than everything, so it
-- would slip past a range check while poisoning ordering and aggregates. It
-- carries no information, so it is dropped rather than clamped.
UPDATE watch_history SET rating = NULL WHERE rating IS NOT NULL AND rating <> rating;
UPDATE favorites    SET rating = NULL WHERE rating IS NOT NULL AND rating <> rating;

UPDATE watch_history SET rating = LEAST(GREATEST(rating, 0), 10)
    WHERE rating IS NOT NULL AND (rating < 0 OR rating > 10);
UPDATE favorites SET rating = LEAST(GREATEST(rating, 0), 10)
    WHERE rating IS NOT NULL AND (rating < 0 OR rating > 10);

UPDATE watch_history SET year = LEAST(GREATEST(year, 0), 9999)
    WHERE year IS NOT NULL AND (year < 0 OR year > 9999);
UPDATE favorites SET year = LEAST(GREATEST(year, 0), 9999)
    WHERE year IS NOT NULL AND (year < 0 OR year > 9999);

UPDATE watch_history SET position_ms = 0 WHERE position_ms < 0;
UPDATE watch_history SET duration_ms  = 0 WHERE duration_ms  < 0;
-- position beyond duration means the client reported a bad duration; treating the
-- row as finished keeps it out of "continue watching" and satisfies the check.
UPDATE watch_history SET position_ms = duration_ms
    WHERE duration_ms > 0 AND position_ms > duration_ms;

UPDATE watch_history SET season  = NULL WHERE season  IS NOT NULL AND season  < 1;
UPDATE watch_history SET episode = NULL WHERE episode IS NOT NULL AND episode < 1;

UPDATE watch_history SET media_type = LOWER(BTRIM(media_type)) WHERE media_type IS NOT NULL;
UPDATE favorites    SET media_type = LOWER(BTRIM(media_type)) WHERE media_type IS NOT NULL;

UPDATE watch_history SET media_type = NULL
    WHERE media_type IS NOT NULL AND media_type <> '' AND media_type !~ '^[a-z0-9_.-]{1,50}$';
UPDATE favorites SET media_type = NULL
    WHERE media_type IS NOT NULL AND media_type <> '' AND media_type !~ '^[a-z0-9_.-]{1,50}$';

-- --- constraints -------------------------------------------------------------
-- Explicitly typed text[][] instead of a bare literal: with unknown-type elements
-- the ARRAY[] expression is only resolved from the FOREACH context.
DO $$
DECLARE
    specs text[][] := ARRAY[
        ARRAY['watch_history', 'chk_watch_history_position_ms', 'position_ms >= 0'],
        ARRAY['watch_history', 'chk_watch_history_duration_ms',  'duration_ms >= 0'],
        ARRAY['watch_history', 'chk_watch_history_progress_ms',  'duration_ms = 0 OR position_ms <= duration_ms'],
        ARRAY['watch_history', 'chk_watch_history_rating',       'rating IS NULL OR (rating >= 0 AND rating <= 10)'],
        ARRAY['watch_history', 'chk_watch_history_year',         'year IS NULL OR (year >= 0 AND year <= 9999)'],
        ARRAY['watch_history', 'chk_watch_history_season',       'season IS NULL OR season >= 1'],
        ARRAY['watch_history', 'chk_watch_history_episode',      'episode IS NULL OR episode >= 1'],
        ARRAY['watch_history', 'chk_watch_history_media_type',   'media_type IS NULL OR media_type = '''' OR media_type ~ ''^[a-z0-9_.-]{1,50}$'''],
        ARRAY['favorites',    'chk_favorites_rating',            'rating IS NULL OR (rating >= 0 AND rating <= 10)'],
        ARRAY['favorites',    'chk_favorites_year',              'year IS NULL OR (year >= 0 AND year <= 9999)'],
        ARRAY['favorites',    'chk_favorites_media_type',        'media_type IS NULL OR media_type = '''' OR media_type ~ ''^[a-z0-9_.-]{1,50}$''']
    ];
    spec text[];
    tbl  text;
    con  text;
    chk  text;
BEGIN
    FOREACH spec SLICE 1 IN ARRAY specs
    LOOP
        tbl := spec[1];
        con := spec[2];
        chk := spec[3];

        IF NOT EXISTS (
            SELECT 1
            FROM pg_constraint c
            JOIN pg_class t ON t.oid = c.conrelid
            WHERE c.conname = con AND t.relname = tbl
        ) THEN
            EXECUTE format('ALTER TABLE %I ADD CONSTRAINT %I CHECK (%s)', tbl, con, chk);
        END IF;
    END LOOP;
END $$;