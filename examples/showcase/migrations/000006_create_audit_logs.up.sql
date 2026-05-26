-- 000006_create_audit_logs.up.sql
-- Append-only compliance log. No updated_at, no foreign keys (records must
-- survive even if the referenced tenant/user is later deleted).
CREATE TABLE audit_logs (
    id          BIGSERIAL   PRIMARY KEY,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    tenant_id   BIGINT      NOT NULL,
    user_id     BIGINT      NOT NULL,
    action      VARCHAR(64) NOT NULL,
    resource    VARCHAR(64) NOT NULL,
    resource_id BIGINT,
    detail      TEXT
);

CREATE INDEX idx_audit_logs_tenant_id ON audit_logs (tenant_id);
CREATE INDEX idx_audit_logs_user_id   ON audit_logs (user_id);
