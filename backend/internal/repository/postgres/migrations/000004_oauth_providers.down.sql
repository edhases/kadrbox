-- 000004_oauth_providers.down.sql
-- WARNING: destructive — removes the OAuth identity columns, so every Telegram /
-- Discord link on existing accounts is lost and those users must sign in again.
ALTER TABLE users
    DROP COLUMN IF EXISTS discord_id,
    DROP COLUMN IF EXISTS telegram_id;