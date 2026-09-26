// Package notification delivers notifications with swappable adapters.
//
// It validates push and SMS shapes, enforces channel rules, and sends through
// log, Twilio, or FCM backends. It does not queue or schedule messages, only
// direct validated delivery.
//
// Type safety: Notifier interface with typed Options plus Adapter enum plus
// Factory. Channel and Priority enums plus Notification, NewNotification,
// Clone, Validate, TwilioOptions, and FCMOptions. Unsupported features fail
// closed with channel-not-supported errors.
//
// DX: Open with Open, custom backends with Register. Options are typed with
// zero-infra defaults for tests, with the log adapter writing instead of
// sending. Config file plus env NOTIFICATION_ADAPTER (no prefix, e.g.
// NOTIFICATION_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.Notification(). Lazy per-service
// singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Notify takes ctx plus the
// notification and respects cancellation. Close releases resources and takes
// no ctx. Only resolved services close.
//
// Errors: sentinel errors, errors.Is compatible, prefixed notification:. Name
// service and field only in errors, with DuplicateError, UnknownAdapterError,
// InvalidAdapterError, InvalidOptionsError, InvalidNotificationError,
// InvalidTargetError, InvalidChannelError, and ChannelNotSupportedError. Target
// values never appear in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices.
// InvalidTargetError carries only a reason, never the target, to avoid PII in
// logs. SMS targets must be E.164. The log adapter echoes targets so disable
// it in production. Auth tokens and key paths are never logged.
//
// Performance: operation timeout defaults to 30s. Validation runs before any
// network call. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init
// wiring.
//
// Example: see ExampleOpen in example_test.go.
package notification
