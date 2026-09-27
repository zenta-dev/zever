package cas

// CompareAndDeleteScript deletes key only when it still holds the expected
// value. It returns 1 on delete, 0 when the key is missing or owned by
// another holder, so delete never steals a successor lease. GET and DEL run
// atomically inside the script in a single Eval round trip.
const CompareAndDeleteScript = `if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
else
  return 0
end`

// CompareAndExpireScript renews key TTL only when it still holds the expected
// value. It returns 1 on renewal, 0 when the key is missing or owned by
// another holder. GET and PEXPIRE run atomically inside the script.
const CompareAndExpireScript = `if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("PEXPIRE", KEYS[1], ARGV[2])
else
  return 0
end`

// CompareAndPersistScript clears key TTL only when it still holds the expected
// value. It returns 1 on persist, 0 when the key is missing or owned by
// another holder. Used when CompareAndExtend gets a non-positive ttl.
const CompareAndPersistScript = `if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("PERSIST", KEYS[1])
else
  return 0
end`
