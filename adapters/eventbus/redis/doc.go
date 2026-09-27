// Package redis is the Redis Pub/Sub adapter for the eventbus battery.
//
// Redis channels map 1:1 to eventbus topics as "<prefix>:<topic>" (default
// prefix "eventbus"). Delivery is at-most-once, fire-and-forget: publishing
// to a topic with no subscribers silently drops the message by design.
//
// Each Subscribe call opens one dedicated go-redis PubSub connection and
// delivers messages synchronously in receive order, so a single handler sees
// messages in publish order while distinct subscribers proceed concurrently.
// A dedicated connection per subscription is preferred over multiplexing many
// topics over one shared PubSub: the Redis server already fans out to every
// connection, and per-sub connections avoid refcount bugs on
// subscribe/unsubscribe races without measurable cost at eventbus scale.
//
// The Redis client itself is shared via the internal/redis Pool singleton and
// is never closed by this adapter; Close only stops deliveries.
//
// Pull API (SubscribeChan/Unsubscribe) is provided via eventbus.Wrap with
// drop-newest-on-full per subscriber.
package redis
