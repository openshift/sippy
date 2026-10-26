-- Recreate the retired tables for a rollback to a version that still
-- contains the Sippy Chat models. Existing data cannot be restored.
CREATE TABLE IF NOT EXISTS chat_conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    "user" TEXT NOT NULL,
    parent_id UUID,
    messages JSONB NOT NULL,
    metadata JSONB
);

CREATE INDEX IF NOT EXISTS idx_chat_conversations_deleted_at
    ON chat_conversations (deleted_at);
CREATE INDEX IF NOT EXISTS idx_chat_conversations_user
    ON chat_conversations ("user");
CREATE INDEX IF NOT EXISTS idx_chat_conversations_parent_id
    ON chat_conversations (parent_id);

CREATE TABLE IF NOT EXISTS chat_ratings (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    rating INTEGER NOT NULL,
    client_id UUID,
    metadata JSONB
);

CREATE INDEX IF NOT EXISTS idx_chat_ratings_deleted_at
    ON chat_ratings (deleted_at);
CREATE INDEX IF NOT EXISTS idx_chat_ratings_client_id
    ON chat_ratings (client_id);
