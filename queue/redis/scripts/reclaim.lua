local stale = redis.call("ZRANGEBYSCORE", KEYS[3], "-inf", ARGV[1], "LIMIT", 0, ARGV[2])
if #stale == 0 then
	return 0
end
local moved = 0
for i = 1, #stale do
	local field = stale[i]
	local raw = redis.call("HGET", KEYS[1], field)
	if raw then
		local ok, msg = pcall(cjson.decode, raw)
		if ok then
			msg["attempt"] = (msg["attempt"] or 0) + 1
			msg["popped_at"] = nil
			redis.call("HDEL", KEYS[1], field)
			redis.call("ZREM", KEYS[3], field)
			local encOk, enc = pcall(cjson.encode, msg)
			if encOk then
				redis.call("RPUSH", KEYS[2], enc)
				moved = moved + 1
			else
				moved = moved + 1
			end
		else
			redis.call("HDEL", KEYS[1], field)
			redis.call("ZREM", KEYS[3], field)
			moved = moved + 1
		end
	else
		redis.call("ZREM", KEYS[3], field)
	end
end
return moved
