-- 000003_create_products.up.sql
-- Products use soft-delete: deleted_at IS NULL means the record is active.
-- DecrStock and all list queries must include WHERE deleted_at IS NULL.
CREATE TABLE products (
    id         BIGSERIAL      PRIMARY KEY,
    created_at TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    tenant_id  BIGINT         NOT NULL REFERENCES tenants (id),
    name       VARCHAR(255)   NOT NULL,
    price      NUMERIC(12, 2) NOT NULL,
    stock      INTEGER        NOT NULL DEFAULT 0,
    category   VARCHAR(100),

    CONSTRAINT products_price_check CHECK (price >= 0),
    CONSTRAINT products_stock_check CHECK (stock >= 0)
);

CREATE INDEX idx_products_tenant_id  ON products (tenant_id);
CREATE INDEX idx_products_category   ON products (category);
CREATE INDEX idx_products_deleted_at ON products (deleted_at);
