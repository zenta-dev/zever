// Package casbin provides a Casbin-backed permission.Checker.
//
// The checker uses an embedded deny-override RBAC model: an explicit deny
// always wins over an allow, and anything without an allow is denied.
// A casbin.SyncedEnforcer backs concurrent use; per-request subject
// groupings are added before enforcement and removed afterwards under a
// mutex with reference counting, so concurrent Can calls are safe.
package casbin
