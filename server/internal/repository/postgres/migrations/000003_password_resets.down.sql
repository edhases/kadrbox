-- 000003_password_resets.down.sql
-- WARNING: destructive — drops the reset token table, so every outstanding
-- password reset link is invalidated.
DROP INDEX IF EXISTS idx_password_resets_user;
DROP INDEX IF EXISTS idx_password_resets_token;
DROP TABLE IF EXISTS password_resets;