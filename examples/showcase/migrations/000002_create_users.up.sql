-- 000002_create_users.up.sql
CREATE TABLE users (
    id             BIGSERIAL    PRIMARY KEY,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    tenant_id      BIGINT       NOT NULL REFERENCES tenants (id),
    email          VARCHAR(255) NOT NULL,
    name           VARCHAR(255) NOT NULL,
    role           VARCHAR(20)  NOT NULL DEFAULT 'buyer',
    oauth_provider VARCHAR(32),
    oauth_sub      VARCHAR(255),
    password_hash  VARCHAR(255),

    CONSTRAINT users_email_key  UNIQUE (email),
    CONSTRAINT users_role_check CHECK (role IN ('admin', 'seller', 'buyer'))
);

CREATE INDEX idx_users_tenant_id ON users (tenant_id);
CREATE INDEX idx_users_oauth_sub ON users (oauth_sub);
