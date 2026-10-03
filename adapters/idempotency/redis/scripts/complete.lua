-- complete.lua: store a completed result, checking the stored fingerprint.
-- KEYS[1] = idempotency key.
-- ARGV[1] = encoded done record (tag + fpLen + fingerprint + result).
-- ARGV[2] = TTL mode ("PX" or "EX") for the fresh-write fallback.
-- ARGV[3] = TTL value for the fresh-write fallback.
-- ARGV[4] = fingerprint to match against the stored record.
-- Returns {""}, {"mismatch"}, or {"decode"}.
local key = KEYS[1]
local done = ARGV[1]
local mode = ARGV[2]
local ttl = ARGV[3]
local fp = ARGV[4]

local tagPending = string.byte("P")
local tagDone = string.byte("D")

local raw = redis.call("GET", key)
if not raw then
	if mode == "PX" then
		redis.call("SET", key, done, "PX", ttl)
	else
		redis.call("SET", key, done, "EX", ttl)
	end
	return {""}
end

if #raw < 3 then
	return {"decode"}
end
local tag = string.byte(raw, 1)
local n = string.byte(raw, 2) * 256 + string.byte(raw, 3)
if #raw < 3 + n then
	return {"decode"}
end
if tag ~= tagPending and tag ~= tagDone then
	return {"decode"}
end
local stored = string.sub(raw, 4, 3 + n)

if stored ~= fp then
	return {"mismatch"}
end

local ok = redis.call("SET", key, done, "XX", "KEEPTTL")
if not ok then
	if mode == "PX" then
		redis.call("SET", key, done, "PX", ttl)
	else
		redis.call("SET", key, done, "EX", ttl)
	end
end
return {""}
