 -- ============================================================                                                                                        
  -- Showcase Application — PostgreSQL DDL
  -- Generated from internal/domain/entities.go                                                                                                            
  -- ============================================================
                                                                                                                                                           
  -- ── tenants ──────────────────────────────────────────────────
  CREATE TABLE tenants (
      id         BIGSERIAL    PRIMARY KEY,
      created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
      updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
      name       VARCHAR(255) NOT NULL,
      plan       VARCHAR(20)  NOT NULL DEFAULT 'free',

      CONSTRAINT tenants_name_key UNIQUE (name),
      CONSTRAINT tenants_plan_check CHECK (plan IN ('free', 'pro', 'enterprise'))
  );

  -- ── users ─────────────────────────────────────────────────────
  CREATE TABLE users (
      id             BIGSERIAL    PRIMARY KEY,
      created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
      updated_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
      tenant_id      BIGINT       NOT NULL REFERENCES tenants(id),
      email          VARCHAR(255) NOT NULL,
      name           VARCHAR(255) NOT NULL,
      role           VARCHAR(20)  NOT NULL DEFAULT 'buyer',
      oauth_provider VARCHAR(32),
      oauth_sub      VARCHAR(255),
      password_hash  VARCHAR(255),

      CONSTRAINT users_email_key UNIQUE (email),
      CONSTRAINT users_role_check CHECK (role IN ('admin', 'seller', 'buyer'))
  );

  CREATE INDEX idx_users_tenant_id  ON users (tenant_id);
  CREATE INDEX idx_users_oauth_sub  ON users (oauth_sub);

  -- ── products ──────────────────────────────────────────────────
  -- Uses soft-delete: deleted_at IS NULL = active record
  CREATE TABLE products (                                                                                                                                  
      id         BIGSERIAL      PRIMARY KEY,
      created_at TIMESTAMPTZ    NOT NULL DEFAULT NOW(),                                                                                                    
      updated_at TIMESTAMPTZ    NOT NULL DEFAULT NOW(),     
      deleted_at TIMESTAMPTZ,                          -- soft delete
      tenant_id  BIGINT         NOT NULL REFERENCES tenants(id),
      name       VARCHAR(255)   NOT NULL,
      price      NUMERIC(12, 2) NOT NULL,
      stock      INTEGER        NOT NULL DEFAULT 0,
      category   VARCHAR(100),

      CONSTRAINT products_price_check CHECK (price >= 0),
      CONSTRAINT products_stock_check CHECK (stock >= 0)
  );

  CREATE INDEX idx_products_tenant_id ON products (tenant_id);
  CREATE INDEX idx_products_category  ON products (category);
  CREATE INDEX idx_products_deleted_at ON products (deleted_at);

  -- ── orders ────────────────────────────────────────────────────
  CREATE TABLE orders (
      id         BIGSERIAL      PRIMARY KEY,
      created_at TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
      updated_at TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
      tenant_id  BIGINT         NOT NULL REFERENCES tenants(id),
      user_id    BIGINT         NOT NULL REFERENCES users(id),
      total      NUMERIC(12, 2) NOT NULL,
      status     VARCHAR(20)    NOT NULL DEFAULT 'pending',

      CONSTRAINT orders_status_check CHECK (
          status IN ('pending', 'confirmed', 'shipped', 'completed', 'cancelled')
      )
  );

  CREATE INDEX idx_orders_tenant_id ON orders (tenant_id);
  CREATE INDEX idx_orders_user_id   ON orders (user_id);
  CREATE INDEX idx_orders_status    ON orders (status);

  -- ── order_items ───────────────────────────────────────────────
  CREATE TABLE order_items (
      id         BIGSERIAL      PRIMARY KEY,
      created_at TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
      updated_at TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
      order_id   BIGINT         NOT NULL REFERENCES orders(id),
      product_id BIGINT         NOT NULL REFERENCES products(id),
      qty        INTEGER        NOT NULL,
      price      NUMERIC(12, 2) NOT NULL,             -- price snapshot at purchase time

      CONSTRAINT order_items_qty_check CHECK (qty > 0)
  );

  CREATE INDEX idx_order_items_order_id   ON order_items (order_id);
  CREATE INDEX idx_order_items_product_id ON order_items (product_id);

  -- ── audit_logs ────────────────────────────────────────────────
  -- No updated_at — append-only compliance log
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