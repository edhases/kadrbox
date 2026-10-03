-- 000002_email_verification.down.sql
-- WARNING: destructive — drops the token table (in-flight verification links
-- stop working) and the is_verified flag.
DROP INDEX IF EXISTS idx_email_verif_user;
DROP INDEX IF EXISTS idx_email_verif_token;
DROP TABLE IF EXISTS email_verifications;
ALTER TABLE users DROP COLUMN IF EXISTS is_verified;