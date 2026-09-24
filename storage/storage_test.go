package storage

import (
	"context"
	"encoding/json/v2"
	"errors"
	"strings"
	"testing"
	"time"
)

type stubStorage struct{}

func (stubStorage) PresignUpload(_ context.Context, _, _, _ string, _ time.Duration) (PresignedURL, error) {
	return PresignedURL{}, nil
}

func (stubStorage) PresignDownload(_ context.Context, _, _ string, _ time.Duration) (PresignedURL, error) {
	return PresignedURL{}, nil
}

func (stubStorage) Exists(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}

func (stubStorage) Delete(_ context.Context, _, _ string) error {
	return nil
}

func (stubStorage) Move(_ context.Context, _, _, _, _ string) error {
	return nil
}

func (stubStorage) Close(_ context.Context) error {
	return nil
}

func (stubStorage) Name() string {
	return "stub"
}

func TestAdapterString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		adapter Adapter
		want    string
	}{
		{AdapterLocal, "local"},
		{AdapterS3, "s3"},
		{AdapterR2, "r2"},
		{Adapter(99), "unknown"},
	}

	for i := range cases {
		if got := cases[i].adapter.String(); got != cases[i].want {
			t.Errorf("String() = %q, want %q", got, cases[i].want)
		}
	}
}

func TestParseAdapter(t *testing.T) {
	t.Parallel()

	wantOK := map[string]Adapter{
		"local": AdapterLocal,
		"s3":    AdapterS3,
		"r2":    AdapterR2,
	}

	for name, want := range wantOK {
		got, err := ParseAdapter(name)
		if err != nil {
			t.Errorf("ParseAdapter(%q) error: %v", name, err)
			continue
		}

		if got != want {
			t.Errorf("ParseAdapter(%q) = %v, want %v", name, got, want)
		}
	}

	_, err := ParseAdapter("bogus")
	if err == nil {
		t.Fatal("ParseAdapter(bogus) expected error")
	}

	if !errors.Is(err, ErrInvalidAdapter) {
		t.Errorf("expected ErrInvalidAdapter, got %v", err)
	}

	var inv *InvalidAdapterError
	if !errors.As(err, &inv) {
		t.Fatalf("expected *InvalidAdapterError, got %T", err)
	}

	if inv.Adapter != "bogus" {
		t.Errorf("Adapter = %q, want %q", inv.Adapter, "bogus")
	}

	if !strings.Contains(err.Error(), `"bogus"`) {
		t.Errorf("Error() = %q, want it to mention bogus", err.Error())
	}
}

func TestRegistryRegisterAndOpen(t *testing.T) {
	if err := Register(Adapter(77), nil); !errors.Is(err, ErrNilFactory) {
		t.Fatalf("Register nil factory: expected ErrNilFactory, got %v", err)
	}

	stub := func(_ Options) (Storage, error) { return stubStorage{}, nil }
	if err := Register(Adapter(77), stub); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := Register(Adapter(77), stub); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate, got %v", err)
	} else {
		var dup *DuplicateError
		if !errors.As(err, &dup) {
			t.Fatalf("expected *DuplicateError, got %T", err)
		}

		if dup.Adapter != Adapter(77) {
			t.Errorf("Adapter = %v, want Adapter(77)", dup.Adapter)
		}

		if !strings.Contains(err.Error(), "unknown") {
			t.Errorf("Error() = %q", err.Error())
		}
	}

	if _, err := Open(Adapter(79), Options{}); !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("expected ErrUnknownAdapter, got %v", err)
	} else {
		var unk *UnknownAdapterError
		if !errors.As(err, &unk) {
			t.Fatalf("expected *UnknownAdapterError, got %T", err)
		}

		if unk.Adapter != Adapter(79) {
			t.Errorf("Adapter = %v, want Adapter(79)", unk.Adapter)
		}

		if !strings.Contains(err.Error(), "forgotten import?") {
			t.Errorf("Error() = %q", err.Error())
		}
	}

	if err := Register(Adapter(78), func(_ Options) (Storage, error) { return nil, ErrExpired }); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if _, err := Open(Adapter(78), Options{}); !errors.Is(err, ErrExpired) {
		t.Fatalf("expected wrapped ErrExpired, got %v", err)
	} else if !strings.Contains(err.Error(), "storage: open") {
		t.Errorf("Error() = %q, want storage: open prefix", err.Error())
	}

	s, err := Open(Adapter(77), Options{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if s.Name() != "stub" {
		t.Errorf("Name() = %q, want stub", s.Name())
	}
}

func TestOptionsValidate(t *testing.T) {
	t.Parallel()

	if err := (Options{URLBase: "ftp://example.com"}).Validate(); err == nil {
		t.Error("bad url_base: expected error")
	} else if !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("bad url_base: expected ErrInvalidOptions, got %v", err)
	}

	badPolicy := Options{Policy: &PolicyConfig{Buckets: map[BucketName]Policy{"bad/name": {}}}}
	if err := badPolicy.Validate(); err == nil {
		t.Error("bad policy: expected error")
	} else if !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("bad policy: expected ErrInvalidOptions, got %v", err)
	}

	if err := (Options{}).Validate(); err != nil {
		t.Errorf("nil policy: %v", err)
	}

	ok := Options{URLBase: "https://example.com", Policy: &PolicyConfig{Default: &Policy{}}}
	if err := ok.Validate(); err != nil {
		t.Errorf("valid options: %v", err)
	}
}

func TestValidateBaseURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		value   string
		wantErr bool
	}{
		{"", false},
		{"https://example.com", false},
		{"http://localhost:8080/x", false},
		{"http://[::1", true},
		{"ftp://example.com", true},
		{"https://", true},
		{"https://u@example.com", true},
		{"https://example.com/a b", true},
		{"https://example.com/a\nb", true},
		{"https://example.com/a\tb", true},
	}

	for i := range cases {
		err := ValidateBaseURL("storage", "url_base", cases[i].value)
		if (err != nil) != cases[i].wantErr {
			t.Errorf("ValidateBaseURL(%q) err=%v, wantErr=%v", cases[i].value, err, cases[i].wantErr)
		}
	}

	if err := ValidateBaseURL("storage", "url_base", "http://[::1"); err == nil ||
		!strings.Contains(err.Error(), "must be a valid URL") {
		t.Errorf("parse error message = %v", err)
	}

	if err := ValidateBaseURL("storage", "url_base", "ftp://example.com"); err == nil ||
		!strings.Contains(err.Error(), "scheme") {
		t.Errorf("scheme error message = %v", err)
	}

	if err := ValidateBaseURL("storage", "url_base", "https://"); err == nil ||
		!strings.Contains(err.Error(), "host") {
		t.Errorf("host error message = %v", err)
	}

	if err := ValidateBaseURL("storage", "url_base", "https://u@example.com"); err == nil ||
		!strings.Contains(err.Error(), "user info") {
		t.Errorf("userinfo error message = %v", err)
	}

	if err := ValidateBaseURL("storage", "url_base", "https://example.com/a b"); err == nil ||
		!strings.Contains(err.Error(), "whitespace") {
		t.Errorf("whitespace error message = %v", err)
	}
}

func TestValidateBucketKey(t *testing.T) {
	t.Parallel()

	if err := ValidateBucketKey("", "a/b.txt"); !errors.Is(err, ErrInvalidBucket) {
		t.Errorf("expected ErrInvalidBucket, got %v", err)
	}

	if err := ValidateBucketKey("mybucket", ""); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("expected ErrInvalidKey, got %v", err)
	}

	if err := ValidateBucketKey("mybucket", "a/b.txt"); err != nil {
		t.Errorf("valid bucket/key: %v", err)
	}
}

func TestPresignExpiry(t *testing.T) {
	t.Parallel()

	if _, err := PresignExpiry(0); !errors.Is(err, ErrExpired) {
		t.Errorf("zero ttl: expected ErrExpired, got %v", err)
	}

	if _, err := PresignExpiry(-time.Second); !errors.Is(err, ErrExpired) {
		t.Errorf("negative ttl: expected ErrExpired, got %v", err)
	}

	if _, err := PresignExpiry(MaxPresignTTL + time.Second); err == nil ||
		!strings.Contains(err.Error(), "exceeds max") {
		t.Errorf("oversize ttl message = %v", err)
	}

	before := time.Now().Unix()

	exp, err := PresignExpiry(time.Minute)
	if err != nil {
		t.Fatalf("valid ttl: %v", err)
	}

	if exp <= before {
		t.Errorf("expiry %d not after %d", exp, before)
	}
}

func TestValidBucket(t *testing.T) {
	t.Parallel()

	cases := []struct {
		bucket string
		want   bool
	}{
		{"", false},
		{".", false},
		{"..", false},
		{"mybucket", true},
		{"UPPER", true},
		{"a-b.c1", true},
		{" padded ", true},
		{"bad_name", false},
		{"a/b", false},
		{"a b", false},
	}

	for i := range cases {
		if got := ValidBucket(cases[i].bucket); got != cases[i].want {
			t.Errorf("ValidBucket(%q) = %v, want %v", cases[i].bucket, got, cases[i].want)
		}
	}
}

func TestValidKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		key  string
		want bool
	}{
		{"", false},
		{"   ", false},
		{"a/../b", false},
		{"a/./b", false},
		{".", false},
		{"..", false},
		{"/abs", false},
		{"../up", false},
		{"a\x00b", false},
		{`a\b`, false},
		{"a/b.txt", true},
		{" a/b ", true},
	}

	for i := range cases {
		if got := ValidKey(cases[i].key); got != cases[i].want {
			t.Errorf("ValidKey(%q) = %v, want %v", cases[i].key, got, cases[i].want)
		}
	}
}

func TestBucketName(t *testing.T) {
	t.Parallel()

	if BucketName("mybucket").String() != "mybucket" {
		t.Errorf("String() = %q", BucketName("mybucket").String())
	}

	if err := BucketName("mybucket").Validate(); err != nil {
		t.Errorf("Validate ok: %v", err)
	}

	if err := BucketName("bad/name").Validate(); !errors.Is(err, ErrInvalidBucket) {
		t.Errorf("expected ErrInvalidBucket, got %v", err)
	}
}

func TestPolicyRuleFor(t *testing.T) {
	t.Parallel()

	p := Policy{
		Read:   Rule{Public: true},
		Write:  Rule{Allow: []Subject{"w"}},
		Update: Rule{Allow: []Subject{"u"}},
		Delete: Rule{Allow: []Subject{"d"}},
	}

	if !p.ruleFor(PermRead).Public {
		t.Error("ruleFor(read) mismatch")
	}

	if got := p.ruleFor(PermWrite).Allow; len(got) != 1 || got[0] != "w" {
		t.Errorf("ruleFor(write) = %v", got)
	}

	if got := p.ruleFor(PermUpdate).Allow; len(got) != 1 || got[0] != "u" {
		t.Errorf("ruleFor(update) = %v", got)
	}

	if got := p.ruleFor(PermDelete).Allow; len(got) != 1 || got[0] != "d" {
		t.Errorf("ruleFor(delete) = %v", got)
	}

	got := p.ruleFor(Perm("bogus"))
	if got.Public || len(got.Allow) != 0 || len(got.Deny) != 0 {
		t.Errorf("ruleFor(bogus) = %+v, want zero Rule", got)
	}
}

func TestPolicyRulePtr(t *testing.T) {
	t.Parallel()

	p := &Policy{}

	if p.rulePtr(PermRead) != &p.Read {
		t.Error("rulePtr(read) mismatch")
	}

	if p.rulePtr(PermWrite) != &p.Write {
		t.Error("rulePtr(write) mismatch")
	}

	if p.rulePtr(PermUpdate) != &p.Update {
		t.Error("rulePtr(update) mismatch")
	}

	if p.rulePtr(PermDelete) != &p.Delete {
		t.Error("rulePtr(delete) mismatch")
	}

	if p.rulePtr(Perm("bogus")) != nil {
		t.Error("rulePtr(bogus) want nil")
	}

	p.rulePtr(PermRead).Public = true
	if !p.Read.Public {
		t.Error("rulePtr mutation did not stick")
	}
}

func TestPolicyPublic(t *testing.T) {
	t.Parallel()

	p := Policy{Read: Rule{Public: true}}

	if !p.Public(PermRead) {
		t.Error("Public(read) want true")
	}

	if p.Public(PermWrite) {
		t.Error("Public(write) want false")
	}
}

func TestPolicyAllow(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		pol  Policy
		perm Perm
		sub  Subject
		auth bool
		want bool
	}{
		{"empty allow denies", Policy{}, PermRead, "alice", true, false},
		{"deny wins", Policy{Read: Rule{Allow: []Subject{"alice"}, Deny: []Subject{"alice"}}}, PermRead, "alice", true, false},
		{"any auth", Policy{Read: Rule{Allow: []Subject{AnySubject}}}, PermRead, "who", true, true},
		{"any anon", Policy{Read: Rule{Allow: []Subject{AnySubject}}}, PermRead, "who", false, false},
		{"anonymous anon", Policy{Read: Rule{Allow: []Subject{Anonymous}}}, PermRead, "", false, true},
		{"anonymous auth", Policy{Read: Rule{Allow: []Subject{Anonymous}}}, PermRead, "x", true, true},
		{"exact", Policy{Read: Rule{Allow: []Subject{"alice"}}}, PermRead, "alice", true, true},
		{"no match", Policy{Read: Rule{Allow: []Subject{"alice"}}}, PermRead, "bob", true, false},
		{"deny any", Policy{Read: Rule{Allow: []Subject{"alice"}, Deny: []Subject{AnySubject}}}, PermRead, "alice", true, false},
		{"deny anonymous", Policy{Read: Rule{Allow: []Subject{Anonymous}, Deny: []Subject{Anonymous}}}, PermRead, "", false, false},
	}

	for i := range cases {
		if got := cases[i].pol.Allow(cases[i].perm, cases[i].sub, cases[i].auth); got != cases[i].want {
			t.Errorf("%s: Allow() = %v, want %v", cases[i].name, got, cases[i].want)
		}
	}
}

func TestSelectUploadPerm(t *testing.T) {
	t.Parallel()

	if got := SelectUploadPerm(Policy{}, true); got != PermWrite {
		t.Errorf("no-check exists: got %q", got)
	}

	if got := SelectUploadPerm(Policy{}, false); got != PermWrite {
		t.Errorf("no-check missing: got %q", got)
	}

	diff := Policy{Write: Rule{Public: true}}

	if got := SelectUploadPerm(diff, true); got != PermUpdate {
		t.Errorf("exists: got %q, want update", got)
	}

	if got := SelectUploadPerm(diff, false); got != PermWrite {
		t.Errorf("missing: got %q, want write", got)
	}
}

func TestPolicyNeedsExistCheck(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		pol  Policy
		want bool
	}{
		{"equal", Policy{}, false},
		{"public differ", Policy{Write: Rule{Public: true}}, true},
		{"allow len differ", Policy{Write: Rule{Allow: []Subject{"a"}}}, true},
		{"allow elem differ", Policy{Write: Rule{Allow: []Subject{"a"}}, Update: Rule{Allow: []Subject{"b"}}}, true},
		{"deny len differ", Policy{Write: Rule{Deny: []Subject{"a"}}}, true},
		{"deny elem differ", Policy{Write: Rule{Deny: []Subject{"a"}}, Update: Rule{Deny: []Subject{"b"}}}, true},
	}

	for i := range cases {
		if got := cases[i].pol.NeedsExistCheck(); got != cases[i].want {
			t.Errorf("%s: NeedsExistCheck() = %v, want %v", cases[i].name, got, cases[i].want)
		}
	}
}

func TestPolicyValidate(t *testing.T) {
	t.Parallel()

	p := &Policy{}
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	if p.Version != "0" {
		t.Errorf("Version = %q, want 0", p.Version)
	}

	p2 := &Policy{Version: " 1 ", Read: Rule{Allow: []Subject{" alice "}, Deny: []Subject{" bob "}}}
	if err := p2.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	if p2.Version != "1" || p2.Read.Allow[0] != "alice" || p2.Read.Deny[0] != "bob" {
		t.Errorf("trim failed: %+v", p2)
	}

	emptyAllow := &Policy{Read: Rule{Allow: []Subject{""}}}
	if err := emptyAllow.Validate(); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("empty allow message = %v", err)
	}

	dupAllow := &Policy{Read: Rule{Allow: []Subject{"a", "a"}}}
	if err := dupAllow.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("dup allow message = %v", err)
	}

	emptyDeny := &Policy{Read: Rule{Deny: []Subject{""}}}
	if err := emptyDeny.Validate(); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("empty deny message = %v", err)
	}

	dupDeny := &Policy{Read: Rule{Deny: []Subject{"a", "a"}}}
	if err := dupDeny.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("dup deny message = %v", err)
	}
}

func TestPolicyValidateNilRule(t *testing.T) {
	old := allPerms
	cp := make([]Perm, 0, len(old)+1)
	cp = append(cp, old...)
	cp = append(cp, Perm("bogus"))
	allPerms = cp

	defer func() { allPerms = old }()

	p := &Policy{}
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestPolicyConfigResolve(t *testing.T) {
	t.Parallel()

	var nilCfg *PolicyConfig
	policies, def, configured, err := nilCfg.Resolve()
	if err != nil || policies != nil || configured || def.Version != "" {
		t.Errorf("nil receiver: got %v %v %v %v", policies, def, configured, err)
	}

	empty := &PolicyConfig{}
	_, _, confEmpty, errEmpty := empty.Resolve()
	if errEmpty != nil || confEmpty {
		t.Errorf("empty config: configured=%v err=%v", confEmpty, errEmpty)
	}

	defOnly := &PolicyConfig{Default: &Policy{}}
	bucketsDef, defPol, confDef, errDef := defOnly.Resolve()
	if errDef != nil || !confDef || defPol.Version != "0" || len(bucketsDef) != 0 {
		t.Errorf("default only: %+v %v %v %v", bucketsDef, defPol, confDef, errDef)
	}

	badDef := &PolicyConfig{Default: &Policy{Read: Rule{Allow: []Subject{"a", "a"}}}}
	if _, _, _, errBadDef := badDef.Resolve(); errBadDef == nil ||
		!strings.Contains(errBadDef.Error(), "default") {
		t.Errorf("bad default message = %v", errBadDef)
	}

	bucketsOk := &PolicyConfig{Buckets: map[BucketName]Policy{"b1": {}}}
	gotBuckets, _, confBuckets, errBuckets := bucketsOk.Resolve()
	if errBuckets != nil || !confBuckets || len(gotBuckets) != 1 {
		t.Errorf("buckets ok: %v %v %v", gotBuckets, confBuckets, errBuckets)
	}

	badName := &PolicyConfig{Buckets: map[BucketName]Policy{"bad/name": {}}}
	if _, _, _, errBadName := badName.Resolve(); !errors.Is(errBadName, ErrInvalidBucket) {
		t.Errorf("expected ErrInvalidBucket, got %v", errBadName)
	}

	badPol := &PolicyConfig{Buckets: map[BucketName]Policy{"good": {Read: Rule{Allow: []Subject{"a", "a"}}}}}
	if _, _, _, errBadPol := badPol.Resolve(); errBadPol == nil ||
		!strings.Contains(errBadPol.Error(), "good") {
		t.Errorf("bad bucket policy message = %v", errBadPol)
	}
}

func TestPolicyConfigUnmarshalJSON(t *testing.T) {
	t.Parallel()

	var cBad PolicyConfig
	if err := cBad.UnmarshalJSON([]byte("{bad")); err == nil {
		t.Error("bad json: expected error")
	}

	var cNull PolicyConfig
	if err := cNull.UnmarshalJSON([]byte(`{"default": null, "bkt": null}`)); err != nil {
		t.Fatalf("null values: %v", err)
	}

	if cNull.Default != nil || len(cNull.Buckets) != 0 {
		t.Errorf("null values not skipped: %+v", cNull)
	}

	var cUnknown PolicyConfig
	if err := cUnknown.UnmarshalJSON([]byte(`{"default": {"bogus": true}}`)); err == nil {
		t.Error("unknown member: expected error")
	}

	var cType PolicyConfig
	if err := cType.UnmarshalJSON([]byte(`{"default": {"read": "nope"}}`)); err == nil ||
		!strings.Contains(err.Error(), "default") {
		t.Errorf("inner type error message = %v", err)
	}

	var cDef PolicyConfig
	if err := cDef.UnmarshalJSON([]byte(`{"default": {"version": "1"}}`)); err != nil {
		t.Fatalf("default key: %v", err)
	}

	if cDef.Default == nil || cDef.Default.Version != "1" {
		t.Errorf("default not parsed: %+v", cDef.Default)
	}

	var cBuckets PolicyConfig
	if err := cBuckets.UnmarshalJSON([]byte(`{"mybucket": {"version": "2"}, "other": {}}`)); err != nil {
		t.Fatalf("bucket keys: %v", err)
	}

	if cBuckets.Buckets["mybucket"].Version != "2" {
		t.Errorf("mybucket = %+v", cBuckets.Buckets["mybucket"])
	}

	if _, ok := cBuckets.Buckets["other"]; !ok {
		t.Error("other bucket missing")
	}

	var cName PolicyConfig
	if err := cName.UnmarshalJSON([]byte(`{"bad/name": {}}`)); err == nil {
		t.Error("bad bucket name: expected error")
	}
}

func TestPolicyConfigMarshalJSON(t *testing.T) {
	t.Parallel()

	empty := PolicyConfig{}
	dataEmpty, errEmpty := json.Marshal(empty)
	if errEmpty != nil {
		t.Fatalf("marshal empty: %v", errEmpty)
	}

	if string(dataEmpty) != "{}" {
		t.Errorf("empty marshal = %s", dataEmpty)
	}

	cfg := PolicyConfig{
		Buckets: map[BucketName]Policy{"a": {Version: "1"}},
		Default: &Policy{Version: "0"},
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if !strings.Contains(string(data), "default") || !strings.Contains(string(data), `"a"`) {
		t.Errorf("marshal = %s", data)
	}

	var back PolicyConfig
	if errBack := json.Unmarshal(data, &back); errBack != nil {
		t.Fatalf("round-trip: %v", errBack)
	}

	if back.Default == nil || back.Default.Version != "0" {
		t.Errorf("default lost: %+v", back.Default)
	}

	if back.Buckets["a"].Version != "1" {
		t.Errorf("bucket lost: %+v", back.Buckets)
	}

	noDef := PolicyConfig{Buckets: map[BucketName]Policy{"a": {Version: "1"}}}
	dataNoDef, errNoDef := json.Marshal(noDef)
	if errNoDef != nil {
		t.Fatalf("marshal no default: %v", errNoDef)
	}

	if strings.Contains(string(dataNoDef), "default") {
		t.Errorf("nil default leaked: %s", dataNoDef)
	}
}

func TestDenyReason(t *testing.T) {
	t.Parallel()

	if got := DenyReason(Policy{}, PermRead, "x", false); got != ReasonAnonymousForbidden {
		t.Errorf("unauthenticated: got %q", got)
	}

	deny := Policy{Read: Rule{Deny: []Subject{"alice"}}}
	if got := DenyReason(deny, PermRead, "alice", true); got != ReasonDenyMatched {
		t.Errorf("deny-matched: got %q", got)
	}

	if got := DenyReason(Policy{}, PermRead, "bob", true); got != ReasonNotInAllow {
		t.Errorf("not-in-allow: got %q", got)
	}
}

func TestMatchSubject(t *testing.T) {
	t.Parallel()

	cases := []struct {
		entry   Subject
		subject Subject
		auth    bool
		want    bool
	}{
		{AnySubject, "x", true, true},
		{AnySubject, "x", false, false},
		{Anonymous, "x", false, true},
		{Anonymous, "x", true, true},
		{"alice", "alice", true, true},
		{"alice", "alice", false, true},
		{"alice", "bob", true, false},
	}

	for i := range cases {
		if got := matchSubject(cases[i].entry, cases[i].subject, cases[i].auth); got != cases[i].want {
			t.Errorf("matchSubject(%q,%q,%v) = %v, want %v",
				cases[i].entry, cases[i].subject, cases[i].auth, got, cases[i].want)
		}
	}
}

func TestDecisionHooks(t *testing.T) {
	defer SetDecisionHook(nil)

	var got []Decision
	SetDecisionHook(func(_ context.Context, d Decision) {
		got = append(got, d)
	})

	ctx := t.Context()
	FireAllow(ctx, PermRead, "bkt", "alice", ReasonOkPublic, "1")
	FireDeny(ctx, PermWrite, "bkt", "bob", ReasonNotInAllow, "2")
	FireDecision(ctx, Decision{
		Allow:   true,
		Perm:    PermDelete,
		Bucket:  "b",
		Subject: "s",
		Reason:  ReasonOk,
		Version: "3",
	})

	if len(got) != 3 {
		t.Fatalf("got %d decisions, want 3", len(got))
	}

	first := got[0]
	if !first.Allow || first.Perm != PermRead || first.Bucket != "bkt" ||
		first.Subject != "alice" || first.Reason != ReasonOkPublic || first.Version != "1" {
		t.Errorf("allow decision = %+v", first)
	}

	second := got[1]
	if second.Allow || second.Perm != PermWrite || second.Bucket != "bkt" ||
		second.Subject != "bob" || second.Reason != ReasonNotInAllow || second.Version != "2" {
		t.Errorf("deny decision = %+v", second)
	}

	third := got[2]
	if !third.Allow || third.Perm != PermDelete || third.Bucket != "b" ||
		third.Subject != "s" || third.Reason != ReasonOk || third.Version != "3" {
		t.Errorf("direct decision = %+v", third)
	}

	SetDecisionHook(nil)
	FireAllow(ctx, PermRead, "bkt", "alice", ReasonOk, "0")
	FireDeny(ctx, PermRead, "bkt", "alice", ReasonOk, "0")
	FireDecision(ctx, Decision{})

	if len(got) != 3 {
		t.Error("nil hook must not fire")
	}
}

func TestSubjectContext(t *testing.T) {
	t.Parallel()

	if Subject("alice").String() != "alice" {
		t.Errorf("Subject String = %q", Subject("alice").String())
	}

	ctx := WithSubject(t.Context(), Subject("alice"))
	s, ok := SubjectFrom(ctx)
	if !ok || s != "alice" {
		t.Errorf("round-trip: got %q %v", s, ok)
	}

	if _, ok := SubjectFrom(t.Context()); ok {
		t.Error("absent subject: want ok=false")
	}

	type otherKey string
	other := context.WithValue(t.Context(), otherKey("k"), "x")
	if _, ok := SubjectFrom(other); ok {
		t.Error("wrong type: want ok=false")
	}
}

func TestPolicyStore(t *testing.T) {
	t.Parallel()

	var s PolicyStore
	if err := s.ResolveFromConfig(nil); err != nil {
		t.Fatalf("nil cfg: %v", err)
	}

	if s.Configured() {
		t.Error("nil cfg: want unconfigured")
	}

	if len(s.Policies()) != 0 {
		t.Errorf("nil cfg policies = %v", s.Policies())
	}

	if s.Default().Version != "" {
		t.Errorf("nil cfg default = %+v", s.Default())
	}

	if s.PolicyFor("missing").Version != "" {
		t.Errorf("nil cfg PolicyFor = %+v", s.PolicyFor("missing"))
	}

	bad := &PolicyConfig{Buckets: map[BucketName]Policy{"bad/name": {}}}

	var sBad PolicyStore
	if err := sBad.ResolveFromConfig(bad); err == nil {
		t.Error("bad cfg: expected error")
	}

	cfg := &PolicyConfig{
		Default: &Policy{Version: "9"},
		Buckets: map[BucketName]Policy{"b1": {Version: "1"}},
	}

	var sOK PolicyStore
	if err := sOK.ResolveFromConfig(cfg); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	if !sOK.Configured() {
		t.Error("want configured")
	}

	if len(sOK.Policies()) != 1 {
		t.Errorf("policies = %v", sOK.Policies())
	}

	if sOK.Default().Version != "9" {
		t.Errorf("default = %+v", sOK.Default())
	}

	if sOK.PolicyFor("b1").Version != "1" {
		t.Errorf("hit = %+v", sOK.PolicyFor("b1"))
	}

	if sOK.PolicyFor("nope").Version != "9" {
		t.Errorf("miss = %+v", sOK.PolicyFor("nope"))
	}
}

func TestErrorTypes(t *testing.T) {
	t.Parallel()

	inv := &InvalidAdapterError{Adapter: "nope"}
	if inv.Error() != `storage: invalid adapter: "nope"` {
		t.Errorf("Error() = %q", inv.Error())
	}

	if !errors.Is(inv, ErrInvalidAdapter) {
		t.Errorf("Is ErrInvalidAdapter failed: %v", inv)
	}

	if !errors.Is(inv.Unwrap(), ErrInvalidAdapter) {
		t.Error("Unwrap mismatch")
	}

	var inv2 *InvalidAdapterError
	if !errors.As(inv, &inv2) {
		t.Errorf("As failed: %T", inv)
	}

	dup := &DuplicateError{Adapter: AdapterLocal}
	if dup.Error() != "storage: duplicate registration: local" {
		t.Errorf("Error() = %q", dup.Error())
	}

	if !errors.Is(dup, ErrDuplicate) {
		t.Errorf("Is ErrDuplicate failed: %v", dup)
	}

	if !errors.Is(dup.Unwrap(), ErrDuplicate) {
		t.Error("Unwrap mismatch")
	}

	var dup2 *DuplicateError
	if !errors.As(dup, &dup2) {
		t.Errorf("As failed: %T", dup)
	}

	unk := &UnknownAdapterError{Adapter: AdapterS3}
	if unk.Error() != "storage: unknown adapter: s3 (forgotten import?)" {
		t.Errorf("Error() = %q", unk.Error())
	}

	if !errors.Is(unk, ErrUnknownAdapter) {
		t.Errorf("Is ErrUnknownAdapter failed: %v", unk)
	}

	if !errors.Is(unk.Unwrap(), ErrUnknownAdapter) {
		t.Error("Unwrap mismatch")
	}

	var unk2 *UnknownAdapterError
	if !errors.As(unk, &unk2) {
		t.Errorf("As failed: %T", unk)
	}

	invOpt := &InvalidOptionsError{Reason: "bad url_base"}
	if invOpt.Error() != "storage: invalid options: bad url_base" {
		t.Errorf("Error() = %q", invOpt.Error())
	}

	if !errors.Is(invOpt, ErrInvalidOptions) {
		t.Errorf("Is ErrInvalidOptions failed: %v", invOpt)
	}

	if !errors.Is(invOpt.Unwrap(), ErrInvalidOptions) {
		t.Error("Unwrap mismatch")
	}

	var invOpt2 *InvalidOptionsError
	if !errors.As(invOpt, &invOpt2) {
		t.Errorf("As failed: %T", invOpt)
	}
}

func TestSentinelMessages(t *testing.T) {
	t.Parallel()

	cases := []struct {
		err  error
		want string
	}{
		{ErrNotFound, "storage: not found"},
		{ErrForbidden, "storage: forbidden"},
		{ErrInvalidBucket, "storage: invalid bucket"},
		{ErrInvalidKey, "storage: invalid key"},
		{ErrExpired, "storage: expired"},
		{ErrTooLarge, "storage: too large"},
		{ErrNilFactory, "storage: nil factory"},
		{ErrDuplicate, "storage: duplicate registration"},
		{ErrUnknownAdapter, "storage: unknown adapter"},
		{ErrInvalidAdapter, "storage: invalid adapter"},
		{ErrInvalidOptions, "storage: invalid options"},
	}

	for i := range cases {
		if cases[i].err.Error() != cases[i].want {
			t.Errorf("Error() = %q, want %q", cases[i].err.Error(), cases[i].want)
		}
	}
}
