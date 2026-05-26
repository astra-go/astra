-- 000001_create_tenants.up.sql
CREATE TABLE tenants (
    id         BIGSERIAL    PRIMARY KEY,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    name       VARCHAR(255) NOT NULL,
    plan       VARCHAR(20)  NOT NULL DEFAULT 'free',

    CONSTRAINT tenants_name_key   UNIQUE (name),
    CONSTRAINT tenants_plan_check CHECK (plan IN ('free', 'pro', 'enterprise'))
);
