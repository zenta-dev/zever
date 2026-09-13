local field = ARGV[1] .. ":" .. ARGV[2]
local existed = redis.call("HEXISTS", KEYS[1], field)
if existed == 1 then
	redis.call("HDEL", KEYS[1], field)
	redis.call("ZREM", KEYS[2], field)
	return 1
end
return 0
