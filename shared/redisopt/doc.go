// Package redisopt holds the stdlib-only Redis option types, validators, and
// redaction helpers shared by every battery's Redis options. It deliberately
// avoids importing go-redis so facade packages can embed these types without
// pulling a Redis client dependency. Client construction lives in
// shared/redisclient, and the nested */redis adapter modules keep using that.
package redisopt
