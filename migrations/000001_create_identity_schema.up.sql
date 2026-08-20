CREATE TABLE users (
    id uuid PRIMARY KEY,
    email text NOT NULL,
    normalized_email text NOT NULL,
    display_name text NOT NULL DEFAULT '',
    status text NOT NULL,
    email_verified_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    version integer NOT NULL DEFAULT 1,
    CONSTRAINT users_status_check
        CHECK (status IN ('pending', 'active', 'blocked', 'deleting')),
    CONSTRAINT users_version_positive_check
        CHECK (version > 0),
    CONSTRAINT users_normalized_email_check
        CHECK (normalized_email = lower(normalized_email))
);

CREATE UNIQUE INDEX users_normalized_email_active_uq
    ON users (normalized_email)
    WHERE deleted_at IS NULL;

CREATE TABLE user_credentials (
    user_id uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    password_hash text NOT NULL,
    password_changed_at timestamptz NOT NULL DEFAULT now(),
    failed_login_count integer NOT NULL DEFAULT 0,
    locked_until timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT user_credentials_failed_login_count_check
        CHECK (failed_login_count >= 0)
);

CREATE TABLE user_sessions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    refresh_token_hash text NOT NULL UNIQUE,
    user_agent text NOT NULL DEFAULT '',
    ip_address inet,
    expires_at timestamptz NOT NULL,
    last_used_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT user_sessions_expiry_check
        CHECK (expires_at > created_at)
);

CREATE INDEX user_sessions_active_by_user_idx
    ON user_sessions (user_id, expires_at DESC)
    WHERE revoked_at IS NULL;

CREATE TABLE one_time_tokens (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    purpose text NOT NULL,
    token_hash text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT one_time_tokens_purpose_check
        CHECK (purpose IN ('verify_email', 'reset_password')),
    CONSTRAINT one_time_tokens_expiry_check
        CHECK (expires_at > created_at)
);

CREATE INDEX one_time_tokens_active_by_user_idx
    ON one_time_tokens (user_id, purpose, expires_at DESC)
    WHERE used_at IS NULL;
