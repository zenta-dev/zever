package mailer

import (
	"context"
)

// Mail is a mail message.
type Mail struct {
	// From is the sender address.
	From Address
	// To lists primary recipients.
	To []Address
	// Cc lists carbon-copy recipients.
	Cc []Address
	// Bcc lists blind-carbon-copy recipients.
	Bcc []Address
	// Subject is the message subject.
	Subject string
	// Body is the plain-text body.
	Body string
	// HTML is the HTML body.
	HTML string
	// Attachments lists message attachments.
	Attachments []Attachment
}

// NewMail builds a Mail with from, recipients, subject, and plain-text body.
func NewMail(from Address, to []Address, subject, body string) Mail {
	return Mail{
		From:    from,
		To:      append([]Address(nil), to...),
		Subject: subject,
		Body:    body,
	}
}

// Clone returns a deep copy of m, duplicating To/Cc/Bcc and attachment content.
func (m Mail) Clone() Mail {
	out := m
	out.To = append([]Address(nil), m.To...)
	out.Cc = append([]Address(nil), m.Cc...)
	out.Bcc = append([]Address(nil), m.Bcc...)
	if m.Attachments != nil {
		out.Attachments = make([]Attachment, len(m.Attachments))
		copy(out.Attachments, m.Attachments)
		for i := range out.Attachments {
			out.Attachments[i].Content = append([]byte(nil), m.Attachments[i].Content...)
		}
	}
	return out
}

// Sender binds a default From address for outgoing mail.
type Sender struct {
	From Address
}

// Send sets mail.From to s.From and delegates to m.
func (s Sender) Send(ctx context.Context, m Mailer, mail *Mail) error {
	mail.From = s.From
	return m.Send(ctx, mail)
}
