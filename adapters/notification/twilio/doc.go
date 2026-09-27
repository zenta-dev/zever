// Package twilio provides a notification.Notifier that sends SMS through
// the Twilio Messages API.
//
// Only notification.ChannelSMS is supported. Title and Data are push-only
// and rejected by core validation; Priority and TTL validate but are not
// transmitted because SMS has no corresponding fields.
package twilio
