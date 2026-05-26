-- 000004_create_orders.up.sql
CREATE TABLE orders (
    id         BIGSERIAL      PRIMARY KEY,
    created_at TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    tenant_id  BIGINT         NOT NULL REFERENCES tenants (id),
    user_id    BIGINT         NOT NULL REFERENCES users (id),
    total      NUMERIC(12, 2) NOT NULL,
    status     VARCHAR(20)    NOT NULL DEFAULT 'pending',

    CONSTRAINT orders_status_check CHECK (
        status IN ('pending', 'confirmed', 'shipped', 'completed', 'cancelled')
    )
);

CREATE INDEX idx_orders_tenant_id ON orders (tenant_id);
CREATE INDEX idx_orders_user_id   ON orders (user_id);
CREATE INDEX idx_orders_status    ON orders (status);
