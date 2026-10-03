-- 000008_token_tables_one_row_per_user.up.sql
--
-- 1. One live token per user in email_verifications / password_resets.
--    The repositories used to DELETE the user's previous row and then INSERT a
--    new one in two separate statements (with the DELETE error discarded), which
--    left a window with no token at all and made a concurrent request for the
--    same user fail on the token UNIQUE constraint. They now use
--    INSERT ... ON CONFLICT (user_id) DO UPDATE, which needs a UNIQUE on user_id.
--
-- 2. Drop the two byte-identical duplicate indexes over the UNIQUE `token`
--    columns (created in 000002/000003). They cost a write amplification on every
--    token insert for no planner benefit.
--
-- 3. Index expires_at so expired tokens can actually be swept — nothing sweeps
--    them today, and both lookups filter on expiry after finding the row.
--
-- The dedupe deletes rows. That is deliberate and matches the pre-existing
-- semantics ("issuing a new token invalidates the previous one"): for each user
-- only the newest token is kept, so at most one link per user stops working, and
-- it is the older one that was already superseded by the mail the user received
-- last. ctid breaks ties between rows written in the same transaction.

DELETE FROM email_verifications e
WHERE e.ctid NOT IN (
    SELECT DISTINCT ON (user_id) ctid
    FROM email_verifications
    ORDER BY user_id, created_at DESC, ctid DESC
);

DELETE FROM password_resets e
WHERE e.ctid NOT IN (
    SELECT DISTINCT ON (user_id) ctid
    FROM password_resets
    ORDER BY user_id, created_at DESC, ctid DESC
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_email_verif_user_uniq  ON email_verifications(user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_password_resets_user_uniq ON password_resets(user_id);

-- Sweeping indexes.
CREATE INDEX IF NOT EXISTS idx_email_verif_expires  ON email_verifications(expires_at);
CREATE INDEX IF NOT EXISTS idx_password_resets_expires ON password_resets(expires_at);

-- Redundant plain indexes on columns that already had one:
--   * idx_email_verif_token / idx_password_resets_token duplicate the UNIQUE
--     constraint on `token`.
--   * idx_email_verif_user / idx_password_resets_user are superseded by the
--     UNIQUE indexes just created on the same column.
DROP INDEX IF EXISTS idx_email_verif_token;
DROP INDEX IF EXISTS idx_password_resets_token;
DROP INDEX IF EXISTS idx_email_verif_user;
DROP INDEX IF EXISTS idx_password_resets_user;