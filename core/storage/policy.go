package storage

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"encoding/json/jsontext"
	"encoding/json/v2"
)

// Perm identifies an operation governed by a policy.
type Perm string

const (
	// PermRead governs downloads and reads.
	PermRead Perm = "read"
	// PermWrite governs creation of new objects.
	PermWrite Perm = "write"
	// PermUpdate governs overwrites of existing objects.
	PermUpdate Perm = "update"
	// PermDelete governs deletions.
	PermDelete Perm = "delete"
)

// Subject identifies a policy principal.
type Subject string

const (
	// AnySubject matches any authenticated subject.
	AnySubject Subject = "*"
	// Anonymous matches everyone, including unauthenticated callers.
	Anonymous Subject = "anonymous"
)

// String returns the string form of Subject.
func (s Subject) String() string {
	return string(s)
}

var allPerms = []Perm{PermRead, PermWrite, PermUpdate, PermDelete}

// Rule is the access rule for a single permission.
type Rule struct {
	// Public allows unsigned access when true.
	Public bool `json:"public,omitempty" yaml:"public,omitempty" mapstructure:"public"`
	// Allow lists subjects granted access.
	Allow []Subject `json:"allow,omitempty" yaml:"allow,omitempty" mapstructure:"allow"`
	// Deny lists subjects refused access; deny wins over allow.
	Deny []Subject `json:"deny,omitempty" yaml:"deny,omitempty" mapstructure:"deny"`
}

// Policy is the per-bucket access policy. Each permission has its own rule.
// Decode with RejectUnknownMembers for strictness; unknown perms then fail
// at decode time.
type Policy struct { //nolint:recvcheck // Validate normalizes in place; readers stay on values so map results remain callable
	// Version is the policy schema version, defaulting to "0" when empty.
	Version string `json:"version,omitempty" yaml:"version,omitempty" mapstructure:"version"`
	// Read governs downloads and reads.
	Read Rule `json:"read,omitempty" yaml:"read,omitempty" mapstructure:"read"`
	// Write governs creation of new objects.
	Write Rule `json:"write,omitempty" yaml:"write,omitempty" mapstructure:"write"`
	// Update governs overwrites of existing objects.
	Update Rule `json:"update,omitempty" yaml:"update,omitempty" mapstructure:"update"`
	// Delete governs deletions.
	Delete Rule `json:"delete,omitempty" yaml:"delete,omitempty" mapstructure:"delete"`
}

func (p Policy) ruleFor(perm Perm) Rule {
	switch perm {
	case PermRead:
		return p.Read
	case PermWrite:
		return p.Write
	case PermUpdate:
		return p.Update
	case PermDelete:
		return p.Delete
	}

	return Rule{}
}

func (p *Policy) rulePtr(perm Perm) *Rule {
	switch perm {
	case PermRead:
		return &p.Read
	case PermWrite:
		return &p.Write
	case PermUpdate:
		return &p.Update
	case PermDelete:
		return &p.Delete
	}

	return nil
}

// Public reports whether perm allows unsigned access.
func (p Policy) Public(perm Perm) bool {
	return p.ruleFor(perm).Public
}

// Allow reports whether subject may exercise perm. Deny wins over allow;
// an empty allow list denies everyone.
func (p Policy) Allow(perm Perm, subject Subject, authenticated bool) bool {
	r := p.ruleFor(perm)

	for _, d := range r.Deny {
		if matchSubject(d, subject, authenticated) {
			return false
		}
	}

	if len(r.Allow) == 0 {
		return false
	}

	for _, a := range r.Allow {
		if matchSubject(a, subject, authenticated) {
			return true
		}
	}

	return false
}

// SelectUploadPerm picks the upload permission from a known existence bit.
// It is the shared tail of every write-vs-update decision; callers keep
// their own existence lookup (stat vs HeadObject vs none).
func SelectUploadPerm(pol Policy, exists bool) Perm {
	if !pol.NeedsExistCheck() {
		return PermWrite
	}

	if exists {
		return PermUpdate
	}

	return PermWrite
}

// NeedsExistCheck reports whether write vs update rules differ, in which case
// callers must stat/HeadObject to pick the permission. When false, callers
// can skip the existence check (perf: saves a syscall / S3 RTT).
func (p Policy) NeedsExistCheck() bool {
	if p.Write.Public != p.Update.Public {
		return true
	}

	if len(p.Write.Allow) != len(p.Update.Allow) {
		return true
	}

	for i := range p.Write.Allow {
		if p.Write.Allow[i] != p.Update.Allow[i] {
			return true
		}
	}

	if len(p.Write.Deny) != len(p.Update.Deny) {
		return true
	}

	for i := range p.Write.Deny {
		if p.Write.Deny[i] != p.Update.Deny[i] {
			return true
		}
	}

	return false
}

// Validate normalizes (trims version/subjects, defaults version to "0") and
// rejects empty/duplicate subject entries.
func (p *Policy) Validate() error {
	p.Version = strings.TrimSpace(p.Version)
	if p.Version == "" {
		p.Version = "0"
	}

	for _, perm := range allPerms {
		r := p.rulePtr(perm)
		if r == nil {
			continue
		}

		for i := range r.Allow {
			r.Allow[i] = Subject(strings.TrimSpace(string(r.Allow[i])))
		}

		for i := range r.Deny {
			r.Deny[i] = Subject(strings.TrimSpace(string(r.Deny[i])))
		}

		if err := validateSubjects(r.Allow, string(perm), "allow"); err != nil {
			return err
		}

		if err := validateSubjects(r.Deny, string(perm), "deny"); err != nil {
			return err
		}
	}

	return nil
}

func validateSubjects(entries []Subject, perm string, field string) error {
	seen := make(map[Subject]struct{}, len(entries))

	for _, e := range entries {
		if e == "" {
			return fmt.Errorf("storage: policy %s.%s contains empty entry", perm, field)
		}

		if _, dup := seen[e]; dup {
			return fmt.Errorf("storage: policy %s.%s contains duplicate %q", perm, field, string(e))
		}

		seen[e] = struct{}{}
	}

	return nil
}

// PolicyConfig is the typed policy option. Nil *PolicyConfig means
// unconfigured (legacy private mode). Buckets keys are bucket names;
// Default applies to unlisted buckets.
type PolicyConfig struct { //nolint:recvcheck // Resolve is nil-safe on pointer; MarshalJSON must stay on value for encoding/json/v2
	// Default is the fallback policy for unlisted buckets.
	Default *Policy
	// Buckets holds per-bucket policies keyed by bucket name.
	Buckets map[BucketName]Policy
}

// Resolve validates bucket names and every policy, returning the effective
// per-bucket map, the default policy, and whether any policy was configured.
func (c *PolicyConfig) Resolve() (map[BucketName]Policy, Policy, bool, error) {
	if c == nil {
		return nil, Policy{}, false, nil
	}

	var def Policy

	configured := false

	if c.Default != nil {
		p := *c.Default
		if err := p.Validate(); err != nil {
			return nil, Policy{}, false, fmt.Errorf("storage: policy default: %w", err)
		}

		def = p
		configured = true
	}

	var buckets map[BucketName]Policy

	if len(c.Buckets) > 0 {
		buckets = make(map[BucketName]Policy, len(c.Buckets))

		for name, p := range c.Buckets {
			if err := name.Validate(); err != nil {
				return nil, Policy{}, false, err
			}

			cp := p
			if err := cp.Validate(); err != nil {
				return nil, Policy{}, false, fmt.Errorf("storage: policy for bucket %q: %w", string(name), err)
			}

			buckets[name] = cp
			configured = true
		}
	}

	return buckets, def, configured, nil
}

// UnmarshalJSON keeps the flat config-file shape:
//
//	{"default": {...}, "bucketA": {...}}
//
// Unknown members are rejected (encoding/json/v2 strict mode).
func (c *PolicyConfig) UnmarshalJSON(b []byte) error {
	var raw map[string]jsontext.Value
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}

	c.Buckets = nil
	c.Default = nil

	for name, msg := range raw {
		if strings.TrimSpace(string(msg)) == "null" {
			continue
		}

		var p Policy
		if err := json.Unmarshal([]byte(msg), &p, json.RejectUnknownMembers(true)); err != nil {
			return fmt.Errorf("storage: policy %q: %w", name, err)
		}

		if name == "default" {
			cp := p
			c.Default = &cp

			continue
		}

		bn := BucketName(name)
		if err := bn.Validate(); err != nil {
			return err
		}

		if c.Buckets == nil {
			c.Buckets = make(map[BucketName]Policy)
		}

		c.Buckets[bn] = p
	}

	return nil
}

// MarshalJSON emits the flat shape UnmarshalJSON accepts.
func (c PolicyConfig) MarshalJSON() ([]byte, error) {
	out := make(map[BucketName]Policy, len(c.Buckets)+1)

	for name, p := range c.Buckets {
		out[name] = p
	}

	if c.Default != nil {
		out["default"] = *c.Default
	}

	return json.Marshal(out)
}

// DenyReason explains why Allow refused perm for subject.
func DenyReason(pol Policy, perm Perm, subject Subject, authenticated bool) Reason {
	if !authenticated {
		return ReasonAnonymousForbidden
	}

	for _, d := range pol.ruleFor(perm).Deny {
		if matchSubject(d, subject, authenticated) {
			return ReasonDenyMatched
		}
	}

	return ReasonNotInAllow
}

func matchSubject(entry Subject, subject Subject, authenticated bool) bool {
	if entry == AnySubject {
		return authenticated
	}

	if entry == Anonymous {
		return true
	}

	return entry == subject
}

// Reason identifies why a policy decision was made.
type Reason string

const (
	// ReasonOk marks an allowed private decision.
	ReasonOk Reason = "ok"
	// ReasonOkPublic marks an allowed public (unsigned) decision.
	ReasonOkPublic Reason = "ok-public"
	// ReasonNotInAllow marks a subject missing from the allow list.
	ReasonNotInAllow Reason = "not-in-allow"
	// ReasonDenyMatched marks a subject matching the deny list.
	ReasonDenyMatched Reason = "deny-matched"
	// ReasonAnonymousForbidden marks an unauthenticated private decision.
	ReasonAnonymousForbidden Reason = "anonymous-forbidden"
	// ReasonPolicyDrift marks detected external bucket-policy changes.
	ReasonPolicyDrift Reason = "policy-drift"
	// ReasonPolicySyncError marks a failed bucket-policy sync.
	ReasonPolicySyncError Reason = "policy-sync-error"
)

// Decision records a policy verdict for the decision hook.
type Decision struct {
	// Allow is the verdict.
	Allow bool
	// Perm is the evaluated permission.
	Perm Perm
	// Bucket is the evaluated bucket.
	Bucket string
	// Subject is the evaluated subject.
	Subject Subject
	// Reason explains the verdict.
	Reason Reason
	// Version is the policy version.
	Version string
}

type decisionHookFunc func(ctx context.Context, d Decision)

var (
	hookMu sync.RWMutex
	hook   decisionHookFunc
)

// SetDecisionHook installs the decision observer, replacing any previous one.
func SetDecisionHook(fn decisionHookFunc) {
	hookMu.Lock()
	defer hookMu.Unlock()

	hook = fn
}

func fireDecisionHook(ctx context.Context, d Decision) {
	hookMu.RLock()

	h := hook

	hookMu.RUnlock()

	if h != nil {
		h(ctx, d)
	}
}

// FireDecision emits d to the decision hook, if one is installed.
func FireDecision(ctx context.Context, d Decision) {
	fireDecisionHook(ctx, d)
}

// WithSubject attaches subject to ctx.
func WithSubject(ctx context.Context, subject Subject) context.Context {
	return context.WithValue(ctx, subjectKey, subject)
}

// SubjectFrom extracts the subject from ctx.
func SubjectFrom(ctx context.Context) (Subject, bool) {
	s, ok := ctx.Value(subjectKey).(Subject)
	return s, ok
}

type subjectKeyType int

const subjectKey subjectKeyType = iota
