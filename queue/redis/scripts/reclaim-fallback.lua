local entries = redis.call("HGETALL", KEYS[1])
local cutoff = tonumber(ARGV[1])
local moved = 0
for i = 1, #entries, 2 do
	local field = entries[i]
	local raw = entries[i + 1]
	local ok, msg = pcall(cjson.decode, raw)
	if not ok then
		redis.call("HDEL", KEYS[1], field)
		redis.call("ZREM", KEYS[3], field)
		moved = moved + 1
	else
		local poppedAt = msg["popped_at"]
		if poppedAt == nil or poppedAt == 0 or poppedAt < cutoff then
			local attempt = msg["attempt"]
			if attempt == nil then
				attempt = 0
			end
			msg["attempt"] = attempt + 1
			msg["popped_at"] = nil
			redis.call("HDEL", KEYS[1], field)
			redis.call("ZREM", KEYS[3], field)
			local encOk, enc = pcall(cjson.encode, msg)
			if encOk then
				redis.call("RPUSH", KEYS[2], enc)
			else
				redis.call("RPUSH", KEYS[2], raw)
			end
			moved = moved + 1
		end
	end
end
return moved
