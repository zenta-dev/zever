package fcm

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"firebase.google.com/go/v4/messaging"

	"github.com/zenta-dev/zever/core/notification"
	sharedfirebase "github.com/zenta-dev/zever/shared/firebase"
	"github.com/zenta-dev/zever/shared/retry"
)

// sendRetryPolicy bounds retries of a transient FCM send failure (network
// blip, FCM 5xx): short exponential backoff from 200ms, capped at 2s, up to
// 3 attempts total. A duplicate push notification on a spurious retry is a
// low-severity nuisance, unlike e.g. a duplicate paid SMS, so this is safe
// to retry unconditionally.
var sendRetryPolicy = retry.Policy{
	BaseDelay:   200 * time.Millisecond,
	Multiplier:  2,
	MaxDelay:    2 * time.Second,
	MaxAttempts: 3,
	JitterMode:  retry.JitterFlat,
	JitterMax:   100 * time.Millisecond,
}

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
	creds := sharedfirebase.Credentials{ProjectID: opts.FCM.ProjectID, ServiceAccount: opts.FCM.ServiceAccount}
	if err := creds.Validate(); err != nil {
		return nil, invalidOptions(err.Error())
	}

	ctx := context.Background()
	app, err := sharedfirebase.NewApp(ctx, creds)
	if err != nil {
		return nil, fmt.Errorf("fcm: init messaging: %w", err)
	}
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
	if err := retry.Do(ctx, sendRetryPolicy, func(ctx context.Context) error {
		_, err := send(ctx, msg)
		return err
	}); err != nil {
		return fmt.Errorf("fcm: send: %w", err)
	}
	return nil
}

// Close shuts down the notifier. It is idempotent and always returns nil.
func (f *notifier) Close() error {
	return nil
}

// validateServiceAccountPath enforces the shared key-file path policy.
// Missing files pass: readability and JSON are checked in New via
// Credentials.Validate. Kept for hermetic unit tests of the path guard.
func validateServiceAccountPath(p string) error {
	if err := sharedfirebase.ValidateServiceAccountPath(p); err != nil {
		return invalidOptions(err.Error())
	}
	return nil
}

// invalidOptions wraps reason as notification.InvalidOptionsError with fcm
// context, preserving errors.Is/As through the chain.
func invalidOptions(reason string) error {
	return fmt.Errorf("fcm: %w", notification.InvalidOptionsError{Reason: reason})
}
