local field = ARGV[1] .. ":" .. ARGV[2]
local raw = redis.call("HGET", KEYS[1], field)
if not raw then
	return 0
end
redis.call("HDEL", KEYS[1], field)
redis.call("ZREM", KEYS[3], field)
local requeue = tonumber(ARGV[3])
if requeue == 1 then
	local ok, msg = pcall(cjson.decode, raw)
	if ok then
		msg["attempt"] = tonumber(ARGV[2]) + 1
		msg["popped_at"] = nil
		local encOk, enc = pcall(cjson.encode, msg)
		if encOk then
			redis.call("RPUSH", KEYS[2], enc)
		end
	end
end
return 1
