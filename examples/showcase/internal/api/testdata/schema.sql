CREATE TABLE IF NOT EXISTS "users" (
  "id" TEXT NOT NULL,
  "email" TEXT NOT NULL UNIQUE,
  "name" VARCHAR(100) NOT NULL,
  "nickname" TEXT,
  "role" TEXT NOT NULL CHECK ("role" IN ('admin', 'member')),
  "password_hash" TEXT NOT NULL,
  "age" INTEGER NOT NULL,
  "credit_cents" INTEGER NOT NULL,
  "rating" REAL NOT NULL,
  "score" REAL NOT NULL,
  "verified" INTEGER NOT NULL,
  "birthday" TEXT NOT NULL,
  "avatar" BLOB NOT NULL,
  "prefs" TEXT NOT NULL,
  "created_at" TEXT NOT NULL,
  PRIMARY KEY ("id")
);
CREATE TABLE IF NOT EXISTS "profiles" (
  "id" TEXT NOT NULL,
  "user_id" TEXT NOT NULL UNIQUE REFERENCES "users" ("id"),
  "bio" VARCHAR(500) NOT NULL,
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
  "sku" VARCHAR(40) NOT NULL UNIQUE,
  "headline" VARCHAR(200) NOT NULL,
  "description" TEXT NOT NULL,
  "price_cents" INTEGER NOT NULL,
  "stock" INTEGER NOT NULL,
  "weight" REAL NOT NULL,
  "featured" INTEGER NOT NULL,
  "created_at" TEXT NOT NULL,
  PRIMARY KEY ("id")
);
CREATE TABLE IF NOT EXISTS "tags" (
  "id" TEXT NOT NULL,
  "name" VARCHAR(30) NOT NULL UNIQUE,
  "created_at" TEXT NOT NULL,
  PRIMARY KEY ("id")
);
CREATE TABLE IF NOT EXISTS "orders" (
  "id" TEXT NOT NULL,
  "user_id" TEXT NOT NULL REFERENCES "users" ("id"),
  "total_cents" INTEGER NOT NULL,
  "status" TEXT NOT NULL CHECK ("status" IN ('pending', 'paid', 'shipped', 'cancelled')),
  "priority" TEXT NOT NULL CHECK ("priority" IN ('low', 'high')),
  "note" TEXT,
  "created_at" TEXT NOT NULL,
  PRIMARY KEY ("id")
);
CREATE TABLE IF NOT EXISTS "order_items" (
  "id" TEXT NOT NULL,
  "order_id" TEXT NOT NULL REFERENCES "orders" ("id") ON DELETE CASCADE,
  "product_id" TEXT NOT NULL REFERENCES "products" ("id"),
  "quantity" INTEGER NOT NULL,
  "price_cents" INTEGER NOT NULL,
  PRIMARY KEY ("id")
);
CREATE TABLE IF NOT EXISTS "reviews" (
  "id" TEXT NOT NULL,
  "product_id" TEXT NOT NULL REFERENCES "products" ("id") ON DELETE CASCADE,
  "user_id" TEXT NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
  "rating" INTEGER NOT NULL,
  "body" VARCHAR(500) NOT NULL,
  "created_at" TEXT NOT NULL,
  PRIMARY KEY ("id")
);
CREATE INDEX IF NOT EXISTS "products_category_id_created_at_idx" ON "products" ("category_id", "created_at");
CREATE UNIQUE INDEX IF NOT EXISTS "products_sku_key" ON "products" ("sku");
CREATE INDEX IF NOT EXISTS "orders_user_id_created_at_idx" ON "orders" ("user_id", "created_at");
CREATE INDEX IF NOT EXISTS "orders_status_idx" ON "orders" ("status");
CREATE INDEX IF NOT EXISTS "order_items_order_id_idx" ON "order_items" ("order_id");
CREATE INDEX IF NOT EXISTS "order_items_product_id_idx" ON "order_items" ("product_id");
CREATE INDEX IF NOT EXISTS "reviews_product_id_created_at_idx" ON "reviews" ("product_id", "created_at");
CREATE INDEX IF NOT EXISTS "reviews_user_id_created_at_idx" ON "reviews" ("user_id", "created_at");

