-- 000005_create_order_items.up.sql
CREATE TABLE order_items (
    id         BIGSERIAL      PRIMARY KEY,
    created_at TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    order_id   BIGINT         NOT NULL REFERENCES orders (id),
    product_id BIGINT         NOT NULL REFERENCES products (id),
    qty        INTEGER        NOT NULL,
    price      NUMERIC(12, 2) NOT NULL,

    CONSTRAINT order_items_qty_check CHECK (qty > 0)
);

CREATE INDEX idx_order_items_order_id   ON order_items (order_id);
CREATE INDEX idx_order_items_product_id ON order_items (product_id);
