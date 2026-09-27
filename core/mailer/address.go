package mailer

import (
	"mime"
	"strings"
)

// Address is a mail address with an optional display name.
type Address struct {
	// Name is the optional display name.
	Name string
	// Address is the addr-spec in local@domain form.
	Address string
}

// String renders the address as `Name <addr>`, Q-encoding a non-ASCII
// name, or as a bare addr when no name is set.
func (a Address) String() string {
	if a.Name == "" {
		return a.Address
	}
	name := a.Name
	if !isASCII(name) {
		name = mime.QEncoding.Encode("utf-8", name)
	}
	return name + " <" + a.Address + ">"
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 127 {
			return false
		}
	}
	return true
}

// Attachment is a mail attachment.
type Attachment struct {
	// Name is the attachment filename.
	Name string
	// Content holds the raw attachment bytes.
	Content []byte
	// Inline marks the attachment for inline display.
	Inline bool
	// ContentID is the MIME content ID for inline references.
	ContentID string
}

// Validate checks the address without using the net/mail parser.
// It rejects header-injection and control characters, bad charset,
// and malformed local@domain shapes.
func (a Address) Validate() error {
	if len(a.Name) > 256 {
		return &InvalidAddressError{Field: "Name", Value: a.Name}
	}
	if hasControl(a.Name) {
		return &InvalidAddressError{Field: "Name", Value: a.Name}
	}

	addr := a.Address
	if len(addr) < 1 || len(addr) > 254 {
		return &InvalidAddressError{Field: "Address", Value: addr}
	}
	if strings.Count(addr, "@") != 1 {
		return &InvalidAddressError{Field: "Address", Value: addr}
	}
	if hasControl(addr) {
		return &InvalidAddressError{Field: "Address", Value: addr}
	}
	if strings.ContainsAny(addr, " <>(),;:\"[]") {
		return &InvalidAddressError{Field: "Address", Value: addr}
	}

	parts := strings.SplitN(addr, "@", 2)
	local, domain := parts[0], parts[1]
	if local == "" || domain == "" {
		return &InvalidAddressError{Field: "Address", Value: addr}
	}
	if !strings.Contains(domain, ".") {
		return &InvalidAddressError{Field: "Address", Value: addr}
	}
	for i := 0; i < len(local); i++ {
		if !isLocalChar(local[i]) {
			return &InvalidAddressError{Field: "Address", Value: addr}
		}
	}
	for i := 0; i < len(domain); i++ {
		if !isDomainChar(domain[i]) {
			return &InvalidAddressError{Field: "Address", Value: addr}
		}
	}
	labels := strings.Split(domain, ".")
	for _, l := range labels {
		if l == "" {
			return &InvalidAddressError{Field: "Address", Value: addr}
		}
	}

	return nil
}

func hasControl(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 32 || s[i] == 127 {
			return true
		}
	}
	return false
}

func isLocalChar(c byte) bool {
	if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' {
		return true
	}
	switch c {
	case '.', '_', '%', '+', '-':
		return true
	}
	return false
}

func isDomainChar(c byte) bool {
	if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' {
		return true
	}
	switch c {
	case '.', '-':
		return true
	}
	return false
}
