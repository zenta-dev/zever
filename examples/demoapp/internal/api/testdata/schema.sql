-- Demoapp schema extracted from `zever db migrate --dry-run --adapter=sqlite`.
-- Regenerate: from examples/demoapp, run
--   zever db migrate --dry-run --adapter=sqlite schema/app.zen
CREATE TABLE IF NOT EXISTS "users" (
  "id" TEXT NOT NULL,
  "email" TEXT NOT NULL UNIQUE,
  "name" VARCHAR(100) NOT NULL,
  "role" TEXT NOT NULL,
  "password_hash" TEXT NOT NULL,
  "created_at" TEXT NOT NULL,
  PRIMARY KEY ("id")
);
CREATE TABLE IF NOT EXISTS "categories" (
  "id" TEXT NOT NULL,
  "name" VARCHAR(50) NOT NULL UNIQUE,
  "description" TEXT NOT NULL,
  "created_at" TEXT NOT NULL,
  PRIMARY KEY ("id")
);
CREATE TABLE IF NOT EXISTS "products" (
  "id" TEXT NOT NULL,
  "category_id" TEXT NOT NULL REFERENCES "categories" ("id"),
  "name" VARCHAR(200) NOT NULL,
  "description" TEXT NOT NULL,
  "price_cents" INTEGER NOT NULL,
  "stock" INTEGER NOT NULL,
  "created_at" TEXT NOT NULL,
  PRIMARY KEY ("id")
);
CREATE TABLE IF NOT EXISTS "orders" (
  "id" TEXT NOT NULL,
  "user_id" TEXT NOT NULL REFERENCES "users" ("id"),
  "total_cents" INTEGER NOT NULL,
  "status" TEXT NOT NULL,
  "created_at" TEXT NOT NULL,
  PRIMARY KEY ("id")
);
CREATE TABLE IF NOT EXISTS "order_items" (
  "id" TEXT NOT NULL,
  "order_id" TEXT NOT NULL REFERENCES "orders" ("id"),
  "product_id" TEXT NOT NULL REFERENCES "products" ("id"),
  "quantity" INTEGER NOT NULL,
  "price_cents" INTEGER NOT NULL,
  PRIMARY KEY ("id")
);
CREATE TABLE IF NOT EXISTS "posts" (
  "id" TEXT NOT NULL,
  "user_id" TEXT NOT NULL REFERENCES "users" ("id"),
  "title" VARCHAR(200) NOT NULL,
  "body" TEXT NOT NULL,
  "published" INTEGER NOT NULL,
  "created_at" TEXT NOT NULL,
  PRIMARY KEY ("id")
);
CREATE TABLE IF NOT EXISTS "comments" (
  "id" TEXT NOT NULL,
  "post_id" TEXT NOT NULL,
  "user_id" TEXT NOT NULL,
  "body" VARCHAR(500) NOT NULL,
  "created_at" TEXT NOT NULL,
  PRIMARY KEY ("id")
);
CREATE INDEX IF NOT EXISTS "products_category_id_idx" ON "products" ("category_id");
CREATE INDEX IF NOT EXISTS "orders_user_id_created_at_idx" ON "orders" ("user_id", "created_at");
CREATE INDEX IF NOT EXISTS "order_items_order_id_idx" ON "order_items" ("order_id");
CREATE INDEX IF NOT EXISTS "order_items_product_id_idx" ON "order_items" ("product_id");
CREATE INDEX IF NOT EXISTS "posts_user_id_created_at_idx" ON "posts" ("user_id", "created_at");
CREATE INDEX IF NOT EXISTS "comments_post_id_created_at_idx" ON "comments" ("post_id", "created_at");
