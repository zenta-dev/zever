package redis

import (
	_ "embed"

	goredis "github.com/redis/go-redis/v9"
)

//go:embed scripts/promote.lua
var _promoteScript string
var promoteScript = goredis.NewScript(_promoteScript)

//go:embed scripts/reclaim.lua
var _reclaimScript string
var reclaimScript = goredis.NewScript(_reclaimScript)

//go:embed scripts/reclaim-fallback.lua
var _reclaimFallbackScript string
var reclaimFallbackScript = goredis.NewScript(_reclaimFallbackScript)

//go:embed scripts/claim.lua
var _claimScript string
var claimScript = goredis.NewScript(_claimScript)

//go:embed scripts/ack.lua
var _ackScript string
var ackScript = goredis.NewScript(_ackScript)

//go:embed scripts/nack.lua
var _nackScript string
var nackScript = goredis.NewScript(_nackScript)
