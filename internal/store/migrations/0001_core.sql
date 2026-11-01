CREATE TABLE tenants (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    plan_id             TEXT,
    billing_month_start INTEGER NOT NULL DEFAULT 1 CHECK (billing_month_start BETWEEN 1 AND 28),
    created_at          BIGINT NOT NULL
);

CREATE TABLE users (
    id                   TEXT PRIMARY KEY,
    tenant_id            TEXT NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    email                TEXT NOT NULL,
    password_hash        TEXT NOT NULL,
    role                 TEXT NOT NULL CHECK (role IN ('admin', 'reviewer')),
    must_change_password INTEGER NOT NULL DEFAULT 0 CHECK (must_change_password IN (0, 1)),
    created_at           BIGINT NOT NULL,
    updated_at           BIGINT NOT NULL,
    UNIQUE (tenant_id, email)
);

CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,
    tenant_id  TEXT NOT NULL,
    user_id    TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at BIGINT NOT NULL,
    created_at BIGINT NOT NULL
);

CREATE TABLE personal_access_tokens (
    id           TEXT PRIMARY KEY,
    tenant_id    TEXT NOT NULL,
    user_id      TEXT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    hash         TEXT NOT NULL UNIQUE,
    last_used_at BIGINT,
    created_at   BIGINT NOT NULL
);

CREATE TABLE channels (
    id         TEXT PRIMARY KEY,
    tenant_id  TEXT NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    UNIQUE (tenant_id, name)
);

CREATE TABLE ingest_keys (
    id         TEXT PRIMARY KEY,
    tenant_id  TEXT NOT NULL,
    channel_id TEXT NOT NULL REFERENCES channels (id) ON DELETE CASCADE,
    prefix     TEXT NOT NULL,
    hash       TEXT NOT NULL UNIQUE,
    created_at BIGINT NOT NULL,
    revoked_at BIGINT
);

-- feedback.channel_id deliberately has no foreign key: a processor batch must
-- never fail because a channel was deleted between ingest and insert.
CREATE TABLE feedback (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    channel_id      TEXT NOT NULL,
    message         TEXT NOT NULL,
    user_id         TEXT,
    metadata        {{json}},
    app_version     TEXT,
    platform        TEXT,
    status          TEXT NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'reviewed', 'actioned', 'ignored')),
    idempotency_key TEXT NOT NULL,
    received_at     BIGINT NOT NULL,
    updated_at      BIGINT NOT NULL,
    updated_by      TEXT,
    CONSTRAINT feedback_tenant_idem_key UNIQUE (tenant_id, idempotency_key)
);

CREATE TABLE tags (
    id         TEXT PRIMARY KEY,
    tenant_id  TEXT NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    created_at BIGINT NOT NULL,
    UNIQUE (tenant_id, name)
);

CREATE TABLE feedback_tags (
    tenant_id   TEXT NOT NULL,
    feedback_id TEXT NOT NULL REFERENCES feedback (id) ON DELETE CASCADE,
    tag_id      TEXT NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
    PRIMARY KEY (feedback_id, tag_id)
);

CREATE TABLE status_changes (
    id          TEXT PRIMARY KEY,
    tenant_id   TEXT NOT NULL,
    feedback_id TEXT NOT NULL REFERENCES feedback (id) ON DELETE CASCADE,
    from_status TEXT NOT NULL CHECK (from_status IN ('new', 'reviewed', 'actioned', 'ignored')),
    to_status   TEXT NOT NULL CHECK (to_status IN ('new', 'reviewed', 'actioned', 'ignored')),
    user_id     TEXT NOT NULL,
    via         TEXT NOT NULL CHECK (via IN ('ui', 'mcp')),
    changed_at  BIGINT NOT NULL
);

CREATE INDEX feedback_tenant_id_status_received_at_idx ON feedback (tenant_id, status, received_at DESC);
CREATE INDEX feedback_tenant_id_channel_id_received_at_idx ON feedback (tenant_id, channel_id, received_at DESC);
CREATE INDEX feedback_tenant_id_user_id_idx ON feedback (tenant_id, user_id);
CREATE INDEX feedback_received_at_idx ON feedback (received_at);
CREATE INDEX feedback_tags_tag_id_idx ON feedback_tags (tag_id);
CREATE INDEX status_changes_feedback_id_changed_at_idx ON status_changes (feedback_id, changed_at);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);
CREATE INDEX ingest_keys_channel_id_idx ON ingest_keys (channel_id);
