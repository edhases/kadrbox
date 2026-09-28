-- 000001_init.up.sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 1. Користувачі (Заміна PocketBase users)
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    username VARCHAR(100) NOT NULL,
    avatar_url TEXT,
    bio TEXT,
    role VARCHAR(20) DEFAULT 'user',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 2. Закладки / Обране (Заміна PocketBase favorites)
CREATE TABLE IF NOT EXISTS favorites (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    media_id VARCHAR(255) NOT NULL,
    provider_id VARCHAR(100) NOT NULL,
    title VARCHAR(500) NOT NULL,
    poster_url TEXT,
    year INT,
    media_type VARCHAR(50),
    rating REAL,
    rating_source VARCHAR(100),
    added_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_user_favorite UNIQUE(user_id, media_id, provider_id)
);
CREATE INDEX IF NOT EXISTS idx_favorites_user_added ON favorites(user_id, added_at DESC);

-- 3. Історія переглядів (збережено season/episode як NULL для фільмів + PostgreSQL 16 UNIQUE NULLS NOT DISTINCT)
CREATE TABLE IF NOT EXISTS watch_history (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    media_id VARCHAR(255) NOT NULL,
    provider_id VARCHAR(100) NOT NULL,
    title VARCHAR(500) NOT NULL,
    poster_url TEXT,
    year INT,
    media_type VARCHAR(50),
    season INT,
    episode INT,
    episode_title VARCHAR(500),
    position_ms BIGINT NOT NULL DEFAULT 0,
    duration_ms BIGINT NOT NULL DEFAULT 0,
    last_stream_url TEXT,
    voiceover VARCHAR(255),
    rating REAL,
    rating_source VARCHAR(100),
    watched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_user_history UNIQUE NULLS NOT DISTINCT (user_id, media_id, provider_id, season, episode)
);
CREATE INDEX IF NOT EXISTS idx_history_user_watched ON watch_history(user_id, watched_at DESC);
CREATE INDEX IF NOT EXISTS idx_history_continue ON watch_history(user_id, position_ms, duration_ms);

-- 4. Серверний кеш метаданих (без зайвого важкого GIN-індексу)
CREATE TABLE IF NOT EXISTS content_cache (
    cache_key VARCHAR(255) PRIMARY KEY,
    provider_id VARCHAR(100) NOT NULL,
    content_type VARCHAR(50) NOT NULL,
    data JSONB NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_content_cache_expires ON content_cache(expires_at);

-- 5. Метадані кімнат Watch Party
CREATE TABLE IF NOT EXISTS watch_party_rooms (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    room_code VARCHAR(10) NOT NULL,
    host_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    media_id VARCHAR(255) NOT NULL,
    title VARCHAR(500) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_active_room_code ON watch_party_rooms(room_code) WHERE is_active = TRUE;
