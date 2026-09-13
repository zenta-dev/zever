local raw = redis.call("LINDEX", KEYS[1], 0)
if not raw then
	return 0
end
local ok, msg = pcall(cjson.decode, raw)
if not ok then
	redis.call("LPOP", KEYS[1])
	return redis.error_reply("cjson decode error: " .. tostring(msg))
end
msg["popped_at"] = tonumber(ARGV[1])
redis.call("LPOP", KEYS[1])
local field = msg["id"] .. ":" .. msg["attempt"]
local encOk, enc = pcall(cjson.encode, msg)
if not encOk then
	return redis.error_reply("cjson encode error: " .. tostring(enc))
end
redis.call("HSET", KEYS[2], field, enc)
redis.call("ZADD", KEYS[3], tonumber(ARGV[1]), field)
return enc
