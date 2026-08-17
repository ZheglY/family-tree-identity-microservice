CREATE TABLE used_refresh_tokens (
    token_hash text PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES user_sessions (id) ON DELETE CASCADE,
    used_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    CONSTRAINT used_refresh_tokens_expiry_check
        CHECK (expires_at > used_at)
);

CREATE INDEX used_refresh_tokens_by_session_idx
    ON used_refresh_tokens (session_id, used_at DESC);
