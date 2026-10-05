-- 000006_users_email_ci_unique.up.sql
--
-- Case-insensitive uniqueness for users.email.
--
-- 000001 created `email VARCHAR(255) UNIQUE NOT NULL`, which treats
-- 'Alice@Example.com' and 'alice@example.com' as two different mailboxes. Two
-- accounts could exist for one real address and a password-reset email could be
-- delivered to whichever row sorted first — handing a live reset token to the
-- wrong account.
--
-- Existing rows are NOT rewritten here. Lowercasing them in this migration would
-- itself create the collisions the index forbids (two rows differing only in
-- case collapse to the same value), and quietly deleting or merging accounts is
-- not a decision a schema migration may take on its own. So instead the migration
-- refuses to run while the data is ambiguous and prints exactly which groups need
-- a human decision; the repository now writes normalised (trimmed + lowercased)
-- addresses, and lookups use LOWER() so existing rows stay reachable.
DO $$
DECLARE
    conflicting text;
BEGIN
    SELECT string_agg(grp, E'\n  ' ORDER BY grp) INTO conflicting
    FROM (
        SELECT LOWER(email) || ' (' || COUNT(*) || ' rows: ' ||
               string_agg(email, ', ' ORDER BY email) || ')' AS grp
        FROM users
        GROUP BY LOWER(email)
        HAVING COUNT(*) > 1
    ) d;

    IF conflicting IS NOT NULL THEN
        RAISE EXCEPTION
            'cannot create a case-insensitive unique index on users.email: '
            'the following addresses exist more than once (differing only in case):
  %

Resolve each group manually (merge or delete the redundant accounts) and retry.
This migration is intentionally non-destructive and will not choose for you.',
            conflicting;
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_lower ON users (LOWER(email));

-- The expression index also serves the exact-match lookup used by
-- GetUserByEmail: LOWER(email) = LOWER($1) is a plain equality on the indexed
-- expression, so it stays an index scan.
COMMENT ON INDEX idx_users_email_lower IS
    'Case-insensitive uniqueness for users.email; 000001_users.email_key only enforced byte equality.';