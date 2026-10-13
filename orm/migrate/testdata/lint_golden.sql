-- lint golden: one statement per rule plus one waived row.
-- absence of DROP TABLE here is deliberate (asserted by TestLintGoldenDDL).
ALTER TABLE "public"."users" DROP COLUMN IF EXISTS "email";
CREATE INDEX IF NOT EXISTS "users_nick_idx" ON "public"."users" ("nick");
ALTER TABLE "public"."users" ALTER COLUMN "age" TYPE bigint;
ALTER TABLE "public"."users" ADD COLUMN IF NOT EXISTS "nick" text NOT NULL;
-- zever:allow:drop_column
ALTER TABLE "public"."users" DROP COLUMN IF EXISTS "legacy_flag";
