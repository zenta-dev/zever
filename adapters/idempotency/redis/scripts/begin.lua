-- begin.lua: atomically reserve key on miss, or classify the existing record.
-- KEYS[1] = idempotency key.
-- ARGV[1] = encoded pending record (tag + fpLen + fingerprint).
-- ARGV[2] = TTL mode ("PX" or "EX"), matching go-redis's duration formatting.
-- ARGV[3] = TTL value (milliseconds for PX, seconds for EX).
-- ARGV[4] = fingerprint to match against the stored record.
-- Returns {replay, result, status}: status "" with replay=false is a
-- reservation miss (caller executes); status "" with replay=true is a done
-- replay; "in_progress"/"mismatch"/"decode" are the fail-closed outcomes.
local key = KEYS[1]
local val = ARGV[1]
local mode = ARGV[2]
local ttl = ARGV[3]
local fp = ARGV[4]

local tagPending = string.byte("P")
local tagDone = string.byte("D")

local ok
if mode == "PX" then
	ok = redis.call("SET", key, val, "NX", "PX", ttl)
else
	ok = redis.call("SET", key, val, "NX", "EX", ttl)
end

if ok then
	return {false, "", ""}
end

local raw = redis.call("GET", key)
if not raw then
	return {false, "", ""}
end

if #raw < 3 then
	return {false, "", "decode"}
end
local tag = string.byte(raw, 1)
local n = string.byte(raw, 2) * 256 + string.byte(raw, 3)
if #raw < 3 + n then
	return {false, "", "decode"}
end
if tag ~= tagPending and tag ~= tagDone then
	return {false, "", "decode"}
end
local stored = string.sub(raw, 4, 3 + n)
local result = string.sub(raw, 4 + n)

if stored ~= fp then
	return {false, "", "mismatch"}
end

if tag == tagPending then
	return {false, "", "in_progress"}
end

return {true, result, ""}
