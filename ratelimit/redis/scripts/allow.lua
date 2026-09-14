-- KEYS[1]: bucket key
-- ARGV[1]: capacity (burst), ARGV[2]: rate (tokens/sec),
-- ARGV[3]: now (float seconds), ARGV[4]: cost (float tokens)
local cap = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local cost = tonumber(ARGV[4])

local data = redis.call('HMGET', KEYS[1], 'tokens', 'last')
local tokens = tonumber(data[1])
local last = tonumber(data[2])

if tokens == nil then
  tokens = cap
end
if last == nil then
  last = now
end

local elapsed = math.max(0, now - last)
tokens = math.min(cap, tokens + elapsed * rate)

local allowed = 0
local retry_ms = 0
if tokens >= cost then
  tokens = tokens - cost
  allowed = 1
else
  retry_ms = math.ceil((cost - tokens) / rate * 1000)
end
last = now

local ttl = math.max(60, math.min(3600, math.ceil(cap / rate) + 1))
redis.call('HSET', KEYS[1], 'tokens', tokens, 'last', last)
redis.call('EXPIRE', KEYS[1], ttl)

return {allowed, math.floor(tokens), retry_ms}
