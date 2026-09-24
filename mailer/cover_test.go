package mailer

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var stubAdapterSeq atomic.Int64

func stubAdapter() Adapter {
	return Adapter(20000 + int(stubAdapterSeq.Add(1)))
}

func TestCoverTypedErrorStrings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"Duplicate", DuplicateError{Adapter: SMTP}, "mailer: duplicate registration: smtp"},
		{"Unknown", UnknownAdapterError{Adapter: Log}, "mailer: unknown adapter: log (forgotten import?)"},
		{"InvalidAdapter", InvalidAdapterError{Adapter: "bogus"}, `mailer: invalid adapter: "bogus"`},
		{"InvalidOptions", InvalidOptionsError{Reason: "host must be non-empty"}, "mailer: invalid options: host must be non-empty"},
		{"InvalidAddress", InvalidAddressError{Field: "From", Value: "bad"}, `mailer: invalid address: From "bad"`},
	}
	for _, c := range cases {
		if got := c.err.Error(); got != c.want {
			t.Errorf("%s Error() = %q want %q", c.name, got, c.want)
		}
	}
}

func TestCoverTypedErrorUnwrap(t *testing.T) {
	t.Parallel()
	if !errors.Is(DuplicateError{Adapter: SMTP}, ErrDuplicate) {
		t.Error("DuplicateError value does not unwrap to ErrDuplicate")
	}
	if !errors.Is(&DuplicateError{Adapter: SMTP}, ErrDuplicate) {
		t.Error("DuplicateError pointer does not unwrap to ErrDuplicate")
	}
	if !errors.Is(UnknownAdapterError{Adapter: SMTP}, ErrUnknownAdapter) {
		t.Error("UnknownAdapterError value does not unwrap to ErrUnknownAdapter")
	}
	if !errors.Is(&UnknownAdapterError{Adapter: SMTP}, ErrUnknownAdapter) {
		t.Error("UnknownAdapterError pointer does not unwrap to ErrUnknownAdapter")
	}
	if !errors.Is(InvalidAdapterError{Adapter: "x"}, ErrInvalidAdapter) {
		t.Error("InvalidAdapterError value does not unwrap to ErrInvalidAdapter")
	}
	if !errors.Is(&InvalidAdapterError{Adapter: "x"}, ErrInvalidAdapter) {
		t.Error("InvalidAdapterError pointer does not unwrap to ErrInvalidAdapter")
	}
	if !errors.Is(InvalidOptionsError{Reason: "x"}, ErrInvalidOptions) {
		t.Error("InvalidOptionsError value does not unwrap to ErrInvalidOptions")
	}
	if !errors.Is(&InvalidOptionsError{Reason: "x"}, ErrInvalidOptions) {
		t.Error("InvalidOptionsError pointer does not unwrap to ErrInvalidOptions")
	}
	if !errors.Is(InvalidAddressError{Field: "F", Value: "v"}, ErrInvalidAddress) {
		t.Error("InvalidAddressError value does not unwrap to ErrInvalidAddress")
	}
	if !errors.Is(&InvalidAddressError{Field: "F", Value: "v"}, ErrInvalidAddress) {
		t.Error("InvalidAddressError pointer does not unwrap to ErrInvalidAddress")
	}
}

func TestCoverRegisterNilFactoryMessage(t *testing.T) {
	a := stubAdapter()
	err := Register(a, nil)
	if !errors.Is(err, ErrNilFactory) {
		t.Fatalf("Register nil err = %v, want ErrNilFactory", err)
	}
	want := "mailer: nil factory for adapter " + a.String()
	if err.Error() != want {
		t.Fatalf("Register nil err = %q want %q", err.Error(), want)
	}
}

func TestCoverRegisterDuplicateMessage(t *testing.T) {
	a := stubAdapter()
	ok := func(Options) (Mailer, error) { return &stubMailer{}, nil }
	if err := Register(a, ok); err != nil {
		t.Fatalf("first Register err = %v", err)
	}
	err := Register(a, ok)
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("second Register err = %v, want ErrDuplicate", err)
	}
	want := "mailer: duplicate registration: " + a.String()
	if err.Error() != want {
		t.Fatalf("dup err = %q want %q", err.Error(), want)
	}
}

func TestCoverOpenSuccess(t *testing.T) {
	a := stubAdapter()
	stub := &stubMailer{}
	if err := Register(a, func(Options) (Mailer, error) { return stub, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	m, err := Open(a, Options{Host: "h.example.com", Port: 587})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	msg := &Mail{To: []Address{{Address: "to@example.com"}}, Subject: "s", Body: "b"}
	if err := m.Send(t.Context(), msg); err != nil {
		t.Fatalf("Send err = %v", err)
	}
	if stub.sent != msg {
		t.Fatalf("stub did not capture sent mail")
	}
	if err := m.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}

func TestCoverOpenFactoryErrorWrap(t *testing.T) {
	a := stubAdapter()
	sentinel := errors.New("cover-boom")
	if err := Register(a, func(Options) (Mailer, error) { return nil, sentinel }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	_, err := Open(a, Options{Host: "h.example.com", Port: 587})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrap of sentinel", err)
	}
	want := "mailer: open " + a.String() + ": " + sentinel.Error()
	if err.Error() != want {
		t.Fatalf("Open err = %q want %q", err.Error(), want)
	}
}

func TestCoverOpenUnknownMessage(t *testing.T) {
	a := stubAdapter()
	_, err := Open(a, Options{Host: "h.example.com", Port: 587})
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("Open unknown err = %v, want ErrUnknownAdapter", err)
	}
	want := "mailer: unknown adapter: " + a.String() + " (forgotten import?)"
	if err.Error() != want {
		t.Fatalf("Open unknown err = %q want %q", err.Error(), want)
	}
}

func TestCoverNewMailFields(t *testing.T) {
	t.Parallel()
	from := Address{Name: "Cover", Address: "cover@example.com"}
	to := []Address{{Address: "a@example.com"}, {Name: "B", Address: "b@example.com"}}
	m := NewMail(from, to, "subj", "body")
	if m.From != from {
		t.Errorf("From = %+v want %+v", m.From, from)
	}
	if len(m.To) != len(to) {
		t.Fatalf("To len = %d want %d", len(m.To), len(to))
	}
	for i := range to {
		if m.To[i] != to[i] {
			t.Errorf("To[%d] = %+v want %+v", i, m.To[i], to[i])
		}
	}
	if m.Subject != "subj" || m.Body != "body" {
		t.Errorf("Subject/Body = %q/%q want subj/body", m.Subject, m.Body)
	}
	if m.HTML != "" || m.Cc != nil || m.Bcc != nil || m.Attachments != nil {
		t.Errorf("NewMail zero fields = %+v, want empty", m)
	}
	to[0].Address = "mutated@example.com"
	if m.To[0].Address == "mutated@example.com" {
		t.Error("NewMail To shares backing array with input")
	}
	empty := NewMail(from, nil, "s", "b")
	if len(empty.To) != 0 {
		t.Errorf("nil To len = %d want 0", len(empty.To))
	}
}

func TestCoverMailCloneNilAttachments(t *testing.T) {
	t.Parallel()
	orig := Mail{
		From:    Address{Address: "from@example.com"},
		Subject: "s",
		Body:    "b",
	}
	cl := orig.Clone()
	if cl.From != orig.From || cl.Subject != orig.Subject || cl.Body != orig.Body {
		t.Fatalf("clone scalars differ: %+v vs %+v", cl, orig)
	}
	if cl.Attachments != nil {
		t.Errorf("nil attachments clone = %+v want nil", cl.Attachments)
	}
	if len(cl.To) != 0 || len(cl.Cc) != 0 || len(cl.Bcc) != 0 {
		t.Errorf("nil slices clone = %+v, want empty", cl)
	}
}

func TestCoverMailCloneAttachmentIsolation(t *testing.T) {
	t.Parallel()
	orig := Mail{
		From: Address{Address: "from@example.com"},
		To:   []Address{{Address: "to@example.com"}},
		Attachments: []Attachment{
			{Name: "a.txt", Content: []byte("hello"), Inline: true, ContentID: "cid-a"},
			{Name: "empty.txt"},
		},
	}
	cl := orig.Clone()
	if len(cl.Attachments) != 2 {
		t.Fatalf("attachments len = %d want 2", len(cl.Attachments))
	}
	if cl.Attachments[0].Name != "a.txt" || string(cl.Attachments[0].Content) != "hello" {
		t.Errorf("attachment[0] = %+v", cl.Attachments[0])
	}
	if !cl.Attachments[0].Inline || cl.Attachments[0].ContentID != "cid-a" {
		t.Errorf("attachment[0] meta = %+v", cl.Attachments[0])
	}
	cl.Attachments[0].Content[0] = 'X'
	cl.Attachments[0].Name = "changed"
	cl.Attachments[1].Content = []byte("new")
	if orig.Attachments[0].Content[0] == 'X' {
		t.Error("Clone Attachment Content shares backing array")
	}
	if orig.Attachments[0].Name == "changed" {
		t.Error("Clone Attachment struct shares backing array")
	}
	if len(orig.Attachments[1].Content) != 0 {
		t.Error("nil Content clone mutation leaked into original")
	}
	cl.To[0].Address = "changed@example.com"
	if orig.To[0].Address == "changed@example.com" {
		t.Error("Clone To shares backing array")
	}
}

func TestCoverSenderSendErrorPassthrough(t *testing.T) {
	t.Parallel()
	from := Address{Address: "svc@example.com"}
	s := Sender{From: from}
	sentinel := errors.New("cover-send-fail")
	stub := &stubMailer{err: sentinel}
	mail := &Mail{To: []Address{{Address: "to@example.com"}}, Subject: "hi", Body: "body"}
	err := s.Send(t.Context(), stub, mail)
	if !errors.Is(err, sentinel) {
		t.Fatalf("Send err = %v, want sentinel", err)
	}
	if mail.From != from {
		t.Fatalf("mail.From = %+v want %+v", mail.From, from)
	}
	if stub.sent != mail {
		t.Fatalf("stub did not capture sent mail on error path")
	}
}

func TestCoverOptionsValidateMatrix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		opts       Options
		wantReason string
	}{
		{"zero", Options{Host: "smtp.example.com", Port: 587}, ""},
		{"boundaryLow", Options{Host: "h.example.com", Port: 1}, ""},
		{"boundaryHigh", Options{Host: "h.example.com", Port: 65535}, ""},
		{"fullCreds", Options{Host: "h.example.com", Port: 587, Username: "u", Password: "p"}, ""},
		{"starttlsCreds", Options{Host: "h.example.com", Port: 587, Username: "u", Password: "p", Encryption: EncryptionSTARTTLS}, ""},
		{"implicitCreds", Options{Host: "h.example.com", Port: 465, Username: "u", Password: "p", Encryption: EncryptionImplicitTLS}, ""},
		{"noneNoCreds", Options{Host: "h.example.com", Port: 25, Encryption: EncryptionNone}, ""},
		{"positiveTimeoutSize", Options{Host: "h.example.com", Port: 587, Timeout: time.Second, MaxMessageSize: 1024}, ""},
		{"emptyHost", Options{Host: "", Port: 587}, "host must be non-empty"},
		{"blankHost", Options{Host: "  ", Port: 587}, "host must be non-empty"},
		{"schemeHost", Options{Host: "smtp://h.example.com", Port: 587}, "host must not contain scheme"},
		{"portZero", Options{Host: "h.example.com", Port: 0}, "port must be 1-65535"},
		{"portNegative", Options{Host: "h.example.com", Port: -25}, "port must be 1-65535"},
		{"portHuge", Options{Host: "h.example.com", Port: 65536}, "port must be 1-65535"},
		{"userOnly", Options{Host: "h.example.com", Port: 587, Username: "u"}, "username and password must be set together"},
		{"passOnly", Options{Host: "h.example.com", Port: 587, Password: "p"}, "username and password must be set together"},
		{"badEncryption", Options{Host: "h.example.com", Port: 587, Encryption: Encryption("TLS")}, "encryption must be starttls, implicit, or none"},
		{"noneUser", Options{Host: "h.example.com", Port: 25, Username: "u", Password: "p", Encryption: EncryptionNone}, "encryption none must not carry credentials"},
		{"nonePassOnly", Options{Host: "h.example.com", Port: 25, Password: "p", Encryption: EncryptionNone}, "username and password must be set together"},
		{"negTimeout", Options{Host: "h.example.com", Port: 587, Timeout: -time.Nanosecond}, "timeout must be >= 0"},
		{"negSize", Options{Host: "h.example.com", Port: 587, MaxMessageSize: -1}, "max_message_size must be >= 0"},
	}
	for _, c := range cases {
		err := c.opts.Validate()
		if c.wantReason == "" {
			if err != nil {
				t.Errorf("%s Validate() = %v, want nil", c.name, err)
			}
			continue
		}
		if !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("%s err = %v, want ErrInvalidOptions", c.name, err)
			continue
		}
		var ioe *InvalidOptionsError
		if !errors.As(err, &ioe) {
			t.Errorf("%s err %T is not *InvalidOptionsError", c.name, err)
			continue
		}
		if ioe.Reason != c.wantReason {
			t.Errorf("%s reason = %q want %q", c.name, ioe.Reason, c.wantReason)
		}
		want := "mailer: invalid options: " + c.wantReason
		if err.Error() != want {
			t.Errorf("%s Error() = %q want %q", c.name, err.Error(), want)
		}
	}
}

func TestCoverOptionsHelpers(t *testing.T) {
	t.Parallel()
	var zero Options
	if zero.encryption() != EncryptionSTARTTLS {
		t.Errorf("zero encryption() = %q want starttls", zero.encryption())
	}
	if zero.timeout() != DefaultTimeout {
		t.Errorf("zero timeout() = %v want %v", zero.timeout(), DefaultTimeout)
	}
	if zero.maxMessageSize() != DefaultMaxMessageSize {
		t.Errorf("zero maxMessageSize() = %d want %d", zero.maxMessageSize(), DefaultMaxMessageSize)
	}
	set := Options{Encryption: EncryptionNone, Timeout: 5 * time.Second, MaxMessageSize: 512}
	if set.encryption() != EncryptionNone {
		t.Errorf("set encryption() = %q want none", set.encryption())
	}
	if set.timeout() != 5*time.Second {
		t.Errorf("set timeout() = %v want 5s", set.timeout())
	}
	if set.maxMessageSize() != 512 {
		t.Errorf("set maxMessageSize() = %d want 512", set.maxMessageSize())
	}
	if (Options{Encryption: EncryptionImplicitTLS}).encryption() != EncryptionImplicitTLS {
		t.Error("implicit encryption() passthrough failed")
	}
	neg := Options{Timeout: -time.Second, MaxMessageSize: -7}
	if neg.timeout() != -time.Second {
		t.Errorf("negative timeout() = %v want -1s", neg.timeout())
	}
	if neg.maxMessageSize() != -7 {
		t.Errorf("negative maxMessageSize() = %d want -7", neg.maxMessageSize())
	}
}

func TestCoverAddressString(t *testing.T) {
	t.Parallel()
	if got := (Address{Address: "a@example.com"}).String(); got != "a@example.com" {
		t.Errorf("bare String() = %q want %q", got, "a@example.com")
	}
	if got := (Address{Name: "Jane Doe", Address: "jane@example.com"}).String(); got != "Jane Doe <jane@example.com>" {
		t.Errorf("named String() = %q", got)
	}
	got := (Address{Name: "Jöhn Döe", Address: "john@example.com"}).String()
	if !strings.Contains(got, "john@example.com") {
		t.Errorf("non-ASCII String() = %q missing addr", got)
	}
	if !strings.Contains(got, "=?utf-8?q?") && !strings.Contains(got, "=?UTF-8?Q?") {
		t.Errorf("non-ASCII String() = %q want Q-encoded name", got)
	}
	if !strings.HasSuffix(got, "<john@example.com>") {
		t.Errorf("non-ASCII String() = %q want angle-addr suffix", got)
	}
}

func TestCoverIsASCII(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want bool
	}{
		{"", true},
		{"Jane", true},
		{"a b-c_d+e", true},
		{"\x7f", true},
		{"Jöhn", false},
		{"日本語", false},
		{"a\x80b", false},
	}
	for _, c := range cases {
		if got := isASCII(c.in); got != c.want {
			t.Errorf("isASCII(%q) = %v want %v", c.in, got, c.want)
		}
	}
}

func TestCoverAddressValidateTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		addr      Address
		wantField string
	}{
		{"validBare", Address{Address: "a@b.co"}, ""},
		{"validNamed", Address{Name: "Jane", Address: "jane+tag@example-domain.com"}, ""},
		{"nameTooLong", Address{Name: strings.Repeat("n", 257), Address: "a@example.com"}, "Name"},
		{"nameControl", Address{Name: "Bad\x7fName", Address: "a@example.com"}, "Name"},
		{"addrEmpty", Address{Address: ""}, "Address"},
		{"addrTooLong", Address{Address: strings.Repeat("a", 250) + "@b.co"}, "Address"},
		{"noAt", Address{Address: "nodomain.example.com"}, "Address"},
		{"twoAt", Address{Address: "a@b@example.com"}, "Address"},
		{"addrControl", Address{Address: "a@exa\x01mple.com"}, "Address"},
		{"space", Address{Address: "a b@example.com"}, "Address"},
		{"angleOpen", Address{Address: "a<b@example.com"}, "Address"},
		{"angleClose", Address{Address: "a>b@example.com"}, "Address"},
		{"parenOpen", Address{Address: "a(b@example.com"}, "Address"},
		{"parenClose", Address{Address: "a)b@example.com"}, "Address"},
		{"comma", Address{Address: "a,b@example.com"}, "Address"},
		{"semicolon", Address{Address: "a;b@example.com"}, "Address"},
		{"colon", Address{Address: "a:b@example.com"}, "Address"},
		{"quote", Address{Address: `a"b@example.com`}, "Address"},
		{"bracketOpen", Address{Address: "a[b@example.com"}, "Address"},
		{"bracketClose", Address{Address: "a]b@example.com"}, "Address"},
		{"emptyLocal", Address{Address: "@example.com"}, "Address"},
		{"emptyDomain", Address{Address: "local@"}, "Address"},
		{"noDot", Address{Address: "a@localhost"}, "Address"},
		{"localBang", Address{Address: "a!b@example.com"}, "Address"},
		{"localSlash", Address{Address: "a/b@example.com"}, "Address"},
		{"domainUnderscore", Address{Address: "a@exam_ple.com"}, "Address"},
		{"domainPlus", Address{Address: "a@exam+ple.com"}, "Address"},
		{"leadingDot", Address{Address: "a@.example.com"}, "Address"},
		{"trailingDot", Address{Address: "a@example.com."}, "Address"},
		{"doubleDot", Address{Address: "a@example..com"}, "Address"},
	}
	for _, c := range cases {
		err := c.addr.Validate()
		if c.wantField == "" {
			if err != nil {
				t.Errorf("%s Validate() = %v, want nil", c.name, err)
			}
			continue
		}
		if !errors.Is(err, ErrInvalidAddress) {
			t.Errorf("%s err = %v, want ErrInvalidAddress", c.name, err)
			continue
		}
		var iae *InvalidAddressError
		if !errors.As(err, &iae) {
			t.Errorf("%s err %T is not *InvalidAddressError", c.name, err)
			continue
		}
		if iae.Field != c.wantField {
			t.Errorf("%s field = %q want %q", c.name, iae.Field, c.wantField)
		}
		if iae.Value != c.addr.Address && iae.Field == "Address" {
			t.Errorf("%s value = %q want %q", c.name, iae.Value, c.addr.Address)
		}
	}
}

func TestCoverHasControl(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"abc", false},
		{"a b", false},
		{"~", false},
		{"\x00", true},
		{"\x1f", true},
		{"a\nb", true},
		{"a\rb", true},
		{"a\tb", true},
		{"a\x7fb", true},
		{"\x7f", true},
		{"é", false},
	}
	for _, c := range cases {
		if got := hasControl(c.in); got != c.want {
			t.Errorf("hasControl(%q) = %v want %v", c.in, got, c.want)
		}
	}
}

func TestCoverLocalChar(t *testing.T) {
	t.Parallel()
	for _, c := range []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789") {
		if !isLocalChar(c) {
			t.Errorf("isLocalChar(%q) = false want true", c)
		}
	}
	for _, c := range []byte("._%+-") {
		if !isLocalChar(c) {
			t.Errorf("isLocalChar(%q) = false want true", c)
		}
	}
	for _, c := range []byte("!#$/=?^`{|}~ @,;:<>\"[]\x00\x7f") {
		if isLocalChar(c) {
			t.Errorf("isLocalChar(%q) = true want false", c)
		}
	}
}

func TestCoverDomainChar(t *testing.T) {
	t.Parallel()
	for _, c := range []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789") {
		if !isDomainChar(c) {
			t.Errorf("isDomainChar(%q) = false want true", c)
		}
	}
	for _, c := range []byte(".-") {
		if !isDomainChar(c) {
			t.Errorf("isDomainChar(%q) = false want true", c)
		}
	}
	for _, c := range []byte("_%+!#$/=?^`{|}~ @,;:<>\"[]\x00\x7f") {
		if isDomainChar(c) {
			t.Errorf("isDomainChar(%q) = true want false", c)
		}
	}
}
