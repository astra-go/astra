-- =============================================================
-- Showcase Application — Seed Data
-- Mirrors db.Seed() in internal/db/db.go
--
-- Run after all migrations:
--   psql $DATABASE_URL -f migrations/seed.sql
--   make migrate-up && psql $DATABASE_URL -f migrations/seed.sql
--
-- Idempotent: wrapped in a transaction with an existence check.
-- Safe to re-run — skips insertion if the "demo" tenant already exists.
-- =============================================================

BEGIN;

-- ── Tenant ────────────────────────────────────────────────────────────────────
INSERT INTO tenants (name, plan)
SELECT 'demo', 'pro'
WHERE NOT EXISTS (SELECT 1 FROM tenants WHERE name = 'demo');

-- ── Users ─────────────────────────────────────────────────────────────────────
-- admin
INSERT INTO users (tenant_id, email, name, role)
SELECT t.id, 'admin@demo.local', 'Demo Admin', 'admin'
FROM   tenants t
WHERE  t.name = 'demo'
  AND  NOT EXISTS (SELECT 1 FROM users WHERE email = 'admin@demo.local');

-- seller
INSERT INTO users (tenant_id, email, name, role)
SELECT t.id, 'seller@demo.local', 'Demo Seller', 'seller'
FROM   tenants t
WHERE  t.name = 'demo'
  AND  NOT EXISTS (SELECT 1 FROM users WHERE email = 'seller@demo.local');

-- buyer
INSERT INTO users (tenant_id, email, name, role)
SELECT t.id, 'buyer@demo.local', 'Demo Buyer', 'buyer'
FROM   tenants t
WHERE  t.name = 'demo'
  AND  NOT EXISTS (SELECT 1 FROM users WHERE email = 'buyer@demo.local');

-- ── Products ──────────────────────────────────────────────────────────────────
INSERT INTO products (tenant_id, name, price, stock, category)
SELECT t.id, 'Widget Pro', 29.99, 100, 'widgets'
FROM   tenants t
WHERE  t.name = 'demo'
  AND  NOT EXISTS (
      SELECT 1 FROM products WHERE tenant_id = t.id AND name = 'Widget Pro'
  );

INSERT INTO products (tenant_id, name, price, stock, category)
SELECT t.id, 'Gadget X', 99.99, 50, 'gadgets'
FROM   tenants t
WHERE  t.name = 'demo'
  AND  NOT EXISTS (
      SELECT 1 FROM products WHERE tenant_id = t.id AND name = 'Gadget X'
  );

INSERT INTO products (tenant_id, name, price, stock, category)
SELECT t.id, 'Doohickey', 9.99, 200, 'misc'
FROM   tenants t
WHERE  t.name = 'demo'
  AND  NOT EXISTS (
      SELECT 1 FROM products WHERE tenant_id = t.id AND name = 'Doohickey'
  );

INSERT INTO products (tenant_id, name, price, stock, category)
SELECT t.id, 'Super Gizmo', 149.99, 30, 'gadgets'
FROM   tenants t
WHERE  t.name = 'demo'
  AND  NOT EXISTS (
      SELECT 1 FROM products WHERE tenant_id = t.id AND name = 'Super Gizmo'
  );

INSERT INTO products (tenant_id, name, price, stock, category)
SELECT t.id, 'Nano Widget', 4.99, 500, 'widgets'
FROM   tenants t
WHERE  t.name = 'demo'
  AND  NOT EXISTS (
      SELECT 1 FROM products WHERE tenant_id = t.id AND name = 'Nano Widget'
  );

-- Low-stock item — useful for testing FindLowStock alerts
INSERT INTO products (tenant_id, name, price, stock, category)
SELECT t.id, 'Rare Part', 299.99, 3, 'misc'
FROM   tenants t
WHERE  t.name = 'demo'
  AND  NOT EXISTS (
      SELECT 1 FROM products WHERE tenant_id = t.id AND name = 'Rare Part'
  );

-- ── Sample orders ─────────────────────────────────────────────────────────────
-- One completed order for the buyer, so the UI has something to show.
WITH buyer AS (
    SELECT id FROM users WHERE email = 'buyer@demo.local'
),
tenant AS (
    SELECT id FROM tenants WHERE name = 'demo'
),
widget AS (
    SELECT id, price FROM products
    WHERE  name = 'Widget Pro'
      AND  tenant_id = (SELECT id FROM tenant)
),
gadget AS (
    SELECT id, price FROM products
    WHERE  name = 'Gadget X'
      AND  tenant_id = (SELECT id FROM tenant)
),
new_order AS (
    INSERT INTO orders (tenant_id, user_id, total, status)
    SELECT
        (SELECT id FROM tenant),
        (SELECT id FROM buyer),
        (SELECT price FROM widget) * 2 + (SELECT price FROM gadget),
        'completed'
    WHERE NOT EXISTS (
        SELECT 1 FROM orders o
        JOIN   users  u ON u.id = o.user_id
        WHERE  u.email = 'buyer@demo.local'
    )
    RETURNING id
)
INSERT INTO order_items (order_id, product_id, qty, price)
SELECT (SELECT id FROM new_order), (SELECT id FROM widget), 2, (SELECT price FROM widget)
WHERE  (SELECT id FROM new_order) IS NOT NULL;

-- Second line item on the same order
WITH buyer AS (
    SELECT id FROM users WHERE email = 'buyer@demo.local'
),
tenant AS (
    SELECT id FROM tenants WHERE name = 'demo'
),
gadget AS (
    SELECT id, price FROM products
    WHERE  name = 'Gadget X'
      AND  tenant_id = (SELECT id FROM tenant)
),
existing_order AS (
    SELECT o.id FROM orders o
    JOIN   users  u ON u.id = o.user_id
    WHERE  u.email = 'buyer@demo.local'
    ORDER  BY o.created_at DESC
    LIMIT  1
)
INSERT INTO order_items (order_id, product_id, qty, price)
SELECT (SELECT id FROM existing_order), (SELECT id FROM gadget), 1, (SELECT price FROM gadget)
WHERE  (SELECT id FROM existing_order) IS NOT NULL
  AND  NOT EXISTS (
      SELECT 1 FROM order_items oi
      WHERE  oi.order_id   = (SELECT id FROM existing_order)
        AND  oi.product_id = (SELECT id FROM gadget)
  );

-- ── Audit log sample ──────────────────────────────────────────────────────────
INSERT INTO audit_logs (tenant_id, user_id, action, resource, resource_id, detail)
SELECT
    t.id,
    u.id,
    'role_assigned',
    'user',
    u.id,
    '{"role":"admin","assigned_by":"system","reason":"initial seed"}'
FROM tenants t
JOIN users   u ON u.tenant_id = t.id AND u.email = 'admin@demo.local'
WHERE t.name = 'demo'
  AND NOT EXISTS (
      SELECT 1 FROM audit_logs al
      JOIN   users uu ON uu.id = al.user_id
      WHERE  uu.email = 'admin@demo.local'
        AND  al.action = 'role_assigned'
  );

COMMIT;
