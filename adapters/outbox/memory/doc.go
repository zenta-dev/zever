// Package memory provides a non-durable outbox.Store that publishes
// synchronously on Record. It is for development and tests only: nothing is
// persisted, so a crash between the business write and the publish loses the
// event. Use the db adapter wherever durability matters.
package memory
