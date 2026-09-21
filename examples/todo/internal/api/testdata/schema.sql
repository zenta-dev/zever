CREATE TABLE IF NOT EXISTS "users" (
  "id" TEXT NOT NULL,
  "email" TEXT NOT NULL UNIQUE,
  "password_hash" TEXT NOT NULL,
  "created_at" TEXT NOT NULL,
  PRIMARY KEY ("id")
);
CREATE TABLE IF NOT EXISTS "notes" (
  "id" TEXT NOT NULL,
  "user_id" TEXT NOT NULL REFERENCES "users" ("id") ON DELETE CASCADE,
  "title" VARCHAR(200) NOT NULL,
  "body" TEXT NOT NULL,
  "done" INTEGER NOT NULL,
  "created_at" TEXT NOT NULL,
  PRIMARY KEY ("id")
);
CREATE INDEX IF NOT EXISTS "notes_user_id_created_at_idx" ON "notes" ("user_id", "created_at");
