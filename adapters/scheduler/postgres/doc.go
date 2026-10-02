// Package postgres provides a durable leased scheduler backend: every
// schedule slot is a row in a lease table, and an instance dispatches a
// tick only after claiming the slot's lease in the database. Expired
// leases are reclaimed on open and on every tick, so a dead instance's
// slots resume on survivors without double-firing while the owner lives.
package postgres
