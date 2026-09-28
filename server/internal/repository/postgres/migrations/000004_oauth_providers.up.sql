-- 000004_oauth_providers.up.sql
-- Підтримка авторизації через Telegram та Discord

ALTER TABLE users 
    ADD COLUMN IF NOT EXISTS telegram_id BIGINT UNIQUE,
    ADD COLUMN IF NOT EXISTS discord_id VARCHAR(64) UNIQUE;
