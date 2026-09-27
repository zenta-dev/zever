// Package redisclient builds shared go-redis clients from the option types
// defined in shared/redisopt. Facade packages must import shared/redisopt
// directly so they do not pull go-redis; only the nested */redis adapter
// modules import this package.
package redisclient
