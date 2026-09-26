// Package rbac provides a role-based permission.Checker.
//
// Rules are indexed by action at construction time. Deny rules are
// evaluated before allow rules; a denial returns an explicit_deny
// decision, a grant returns allow, and no match returns implicit_deny.
package rbac
