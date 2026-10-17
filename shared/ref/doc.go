// Package ref provides the comparable typed key that names one wired
// service instance: a battery interface T plus its Options O, addressed by
// service and instance name.
//
// It is a dependency leaf on purpose: core/<battery> packages cannot import
// container (container already imports them), and config cannot import
// container either, so the key type both sides agree on must live below
// them. The container resolves a Ref to exactly one lazily built instance;
// two Refs with equal Service and Name denote the same instance, so Ref is
// comparable and usable as a map key.
package ref
