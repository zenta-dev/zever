// Package eventbus defines a unified messaging facade with swappable adapters.
//
// The push API (Publish/Subscribe) delivers each message to a handler
// function. The pull API (SubscribeChan/Unsubscribe) delivers each message to
// a buffered Go channel instead, so callers can range over messages with
// backpressure: when a pull channel is full the newest message is dropped for
// that subscriber while other subscribers are unaffected.
//
// Every Message carries a ReceivedAt timestamp stamped at publish time.
// The Redis wire envelope (id/payload/headers) is unchanged; cross-process
// messages stamp ReceivedAt at decode time instead.
package eventbus
