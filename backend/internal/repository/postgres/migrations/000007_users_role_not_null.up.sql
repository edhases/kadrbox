-- 000007_users_role_not_null.up.sql
--
-- users.role was `VARCHAR(20) DEFAULT 'user'` with no NOT NULL. A row written
-- without an explicit role (any INSERT that omits the column still gets the
-- default, but a column added by ALTER/UPDATE, or a future code path that passes
-- an explicit NULL) could hold NULL, and that NULL was copied straight into the
-- JWT `role` claim — producing a token that no role check can reason about.
--
-- Backfill first, then constrain. NULL and the empty string both mean "no role
-- was ever assigned" and become the safe default; a *recognised* role is never
-- overwritten.
UPDATE users SET role = 'user' WHERE role IS NULL OR btrim(role) = '';

ALTER TABLE users ALTER COLUMN role SET DEFAULT 'user';
ALTER TABLE users ALTER COLUMN role SET NOT NULL;

-- Enumerated roles. user/admin are the only values the codebase issues or
-- accepts. Added NOT VALID so that an unexpected legacy value cannot block the
-- deploy, then validated only when the data allows it.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint c
        JOIN pg_class t ON t.oid = c.conrelid
        WHERE c.conname = 'chk_users_role' AND t.relname = 'users'
    ) THEN
        ALTER TABLE users
            ADD CONSTRAINT chk_users_role CHECK (role IN ('user', 'admin')) NOT VALID;

        IF EXISTS (SELECT 1 FROM users WHERE role NOT IN ('user', 'admin')) THEN
            RAISE NOTICE
                'chk_users_role left NOT VALID: pre-existing rows hold an unexpected role (%). New writes are checked; review with: SELECT id, email, role FROM users WHERE role <> ALL(''{"user","admin"}'');',
                (SELECT string_agg(role, ', ') FROM (SELECT DISTINCT role FROM users WHERE role NOT IN ('user', 'admin')) s);
        ELSE
            ALTER TABLE users VALIDATE CONSTRAINT chk_users_role;
        END IF;
    END IF;
END $$;