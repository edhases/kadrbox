-- 000007_users_role_not_null.down.sql
-- Removes the enum CHECK and the NOT NULL, so NULL roles (and therefore NULL
-- `role` JWT claims) become possible again. The backfill is not undone: rows that
-- were NULL are now 'user', which is the value they should have had.
ALTER TABLE users DROP CONSTRAINT IF EXISTS chk_users_role;
ALTER TABLE users ALTER COLUMN role DROP NOT NULL;