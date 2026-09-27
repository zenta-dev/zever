local rows = redis.call("ZRANGEBYSCORE", KEYS[1], "-inf", ARGV[1], "LIMIT", 0, ARGV[2])
if #rows == 0 then
	return 0
end
for i = 1, #rows do
	redis.call("ZREM", KEYS[1], rows[i])
	redis.call("RPUSH", KEYS[2], rows[i])
end
return #rows
