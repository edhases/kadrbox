-- 000008_token_tables_one_row_per_user.down.sql
-- Re-creates the redundant indexes and the plain (non-unique) user_id indexes.
-- The dedupe is NOT undone: the extra token rows it removed are gone, which is
-- harmless because only the newest token per user was ever redeemable.
CREATE INDEX IF NOT EXISTS idx_email_verif_token  ON email_verifications(token);
CREATE INDEX IF NOT EXISTS idx_password_resets_token ON password_resets(token);
CREATE INDEX IF NOT EXISTS idx_email_verif_user   ON email_verifications(user_id);
CREATE INDEX IF NOT EXISTS idx_password_resets_user ON password_resets(user_id);
DROP INDEX IF EXISTS idx_email_verif_expires;
DROP INDEX IF EXISTS idx_password_resets_expires;
DROP INDEX IF EXISTS idx_email_verif_user_uniq;
DROP INDEX IF EXISTS idx_password_resets_user_uniq;