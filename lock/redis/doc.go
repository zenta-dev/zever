// Package redis provides a Redis-backed lock.Locker built on SET NX PX
// leases with Lua compare-and-swap release and extend, so only the holder
// can unlock or renew a key.
package redis
