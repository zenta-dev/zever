CREATE TABLE IF NOT EXISTS "users" (
  "id" TEXT NOT NULL,
  "email" TEXT NOT NULL UNIQUE,
  "password_hash" TEXT NOT NULL,
  "created_at" TEXT NOT NULL,
  PRIMARY KEY ("id")
);
CREATE TABLE IF NOT EXISTS "spaces" (
  "id" TEXT NOT NULL,
  "host_id" TEXT NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
  "title" VARCHAR(200) NOT NULL,
  "description" TEXT NOT NULL,
  "lat" REAL NOT NULL,
  "lng" REAL NOT NULL,
  "price_cents" INTEGER NOT NULL,
  "created_at" TEXT NOT NULL,
  PRIMARY KEY ("id")
);
CREATE TABLE IF NOT EXISTS "bookings" (
  "id" TEXT NOT NULL,
  "space_id" TEXT NOT NULL REFERENCES "spaces" ("id") ON DELETE CASCADE,
  "guest_id" TEXT NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
  "start_date" TEXT NOT NULL,
  "end_date" TEXT NOT NULL,
  "status" TEXT NOT NULL,
  "created_at" TEXT NOT NULL,
  PRIMARY KEY ("id")
);
CREATE TABLE IF NOT EXISTS "reviews" (
  "id" TEXT NOT NULL,
  "space_id" TEXT NOT NULL REFERENCES "spaces" ("id") ON DELETE CASCADE,
  "guest_id" TEXT NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
  "rating" INTEGER NOT NULL,
  "body" TEXT NOT NULL,
  "created_at" TEXT NOT NULL,
  PRIMARY KEY ("id")
);
CREATE INDEX IF NOT EXISTS "spaces_host_id_created_at_idx" ON "spaces" ("host_id", "created_at");
