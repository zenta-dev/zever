package fcm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"

	"github.com/zenta-dev/zever/notification"
)

// notifier delivers push notifications via Firebase Cloud Messaging.
type notifier struct {
	client *messaging.Client
	// send is the injectable transport. Nil means client.Send.
	// Tests inject a fake; no live network is used in tests.
	send func(ctx context.Context, msg *messaging.Message) (string, error)
}

var _ notification.Notifier = (*notifier)(nil)

// New validates opts and returns a Notifier sending push via FCM.
// It requires Options.FCM.ProjectID and Options.FCM.ServiceAccount.
// Failures carry notification.ErrInvalidOptions for bad options.
func New(opts notification.Options) (notification.Notifier, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("fcm: %w", err)
	}
	if opts.FCM.ProjectID == "" {
		return nil, fmt.Errorf("fcm: %w: project id is required",
			notification.InvalidOptionsError{Reason: "fcm project id is required"})
	}
	if opts.FCM.ServiceAccount == "" {
		return nil, fmt.Errorf("fcm: %w: service account is required",
			notification.InvalidOptionsError{Reason: "fcm service account is required"})
	}
	if err := validateServiceAccountPath(opts.FCM.ServiceAccount); err != nil {
		return nil, err
	}

	ctx := context.Background()
	// NewApp is lazy and never loads the key file here: with a non-nil
	// Config it always returns a nil error (SDK-verified), so the error
	// branch is pruned. Load failures surface in Messaging below, and the
	// constructor stays fail-closed.
	app, _ := firebase.NewApp(ctx,
		&firebase.Config{ProjectID: opts.FCM.ProjectID},
		option.WithAuthCredentialsFile(option.ServiceAccount, opts.FCM.ServiceAccount))
	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("fcm: init messaging: %w", err)
	}
	return &notifier{client: client}, nil
}

// Notify validates n and sends it as a push message via FCM.
func (f *notifier) Notify(ctx context.Context, n *notification.Notification) error {
	if n == nil {
		return notification.ErrNilNotification
	}
	if err := n.Validate(); err != nil {
		return err
	}
	if n.Channel != notification.ChannelPush {
		return fmt.Errorf("fcm: %w: channel %q",
			notification.ErrChannelNotSupported, string(n.Channel))
	}

	msg := &messaging.Message{
		//nolint:staticcheck // Token targets device registration tokens; Fid is a different identifier (installation ID) and would break sends.
		Token: n.Target,
		Notification: &messaging.Notification{
			Title: n.Title,
			Body:  n.Body,
		},
	}
	if len(n.Data) > 0 {
		msg.Data = n.Data
	}

	if n.Priority == notification.PriorityHigh {
		msg.Android = &messaging.AndroidConfig{Priority: "high"}
		msg.APNS = &messaging.APNSConfig{Headers: map[string]string{"apns-priority": "10"}}
	} else {
		msg.Android = &messaging.AndroidConfig{Priority: "normal"}
		msg.APNS = &messaging.APNSConfig{Headers: map[string]string{"apns-priority": "5"}}
	}

	if n.TTL > 0 {
		ttl := n.TTL
		msg.Android.TTL = &ttl
		msg.APNS.Headers["apns-expiration"] = strconv.FormatInt(time.Now().Add(ttl).Unix(), 10)
	}

	send := f.send
	if send == nil {
		if f.client == nil {
			return ErrNotConfigured
		}
		send = f.client.Send
	}
	if _, err := send(ctx, msg); err != nil {
		return fmt.Errorf("fcm: send: %w", err)
	}
	return nil
}

// Close shuts down the notifier. It is idempotent and always returns nil.
func (f *notifier) Close() error {
	return nil
}

// validateServiceAccountPath enforces a strict key-file path policy:
// non-empty, clean (Clean is a no-op), no ".." element, .json extension,
// and must not resolve to a directory. Missing files pass: readability
// is checked later by the SDK in New.
func validateServiceAccountPath(p string) error {
	invalid := func(reason string) error {
		return fmt.Errorf("fcm: %w: service account %q: %s",
			notification.InvalidOptionsError{Reason: reason}, p, reason)
	}
	if p == "" {
		return invalid("path is required")
	}
	if filepath.Clean(p) != p {
		return invalid("path is not clean")
	}
	for _, el := range strings.Split(p, string(filepath.Separator)) {
		if el == ".." {
			return invalid("path contains traversal")
		}
	}
	if !strings.EqualFold(filepath.Ext(p), ".json") {
		return invalid("path must have .json extension")
	}
	if info, err := os.Stat(p); err == nil && info.IsDir() {
		return invalid("path is a directory")
	}
	return nil
}
