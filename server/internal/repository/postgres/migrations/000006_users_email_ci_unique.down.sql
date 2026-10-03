-- 000006_users_email_ci_unique.down.sql
-- Drops the case-insensitive uniqueness. Existing rows are untouched, so the
-- database can afterwards hold two accounts for one mailbox again — reverting
-- re-opens exactly the hole this migration closed.
DROP INDEX IF EXISTS idx_users_email_lower;