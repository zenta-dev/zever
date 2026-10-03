package redis

import (
	_ "embed"

	goredis "github.com/redis/go-redis/v9"
)

//go:embed scripts/begin.lua
var _beginScript string
var beginScript = goredis.NewScript(_beginScript)

//go:embed scripts/complete.lua
var _completeScript string
var completeScript = goredis.NewScript(_completeScript)
