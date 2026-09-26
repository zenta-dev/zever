package redis

import (
	"context"

	goredis "github.com/redis/go-redis/v9"

	"github.com/zenta-dev/zever/eventbus"
)

func (a *adapter) deliver(ctx context.Context, sub *subscription, handler eventbus.Handler) {
	defer a.wg.Done()

	ch := sub.ps.Channel(goredis.WithChannelSize(a.buffer))

	for {
		select {
		case <-sub.stop:
			return
		case m, ok := <-ch:
			if !ok {
				return
			}

			// Single-channel invariant: sub.ps comes only from
			// a.client.Subscribe(ctx, channel) below (no pattern-subscribe
			// path exists in this file), so m.Channel always equals
			// sub.channel and no skip check is needed.

			if len(m.Payload) > eventbus.MaxMessageSize {
				continue
			}

			msg, err := decodeMessage(sub.topic, []byte(m.Payload))
			if err != nil {
				continue
			}

			a.invoke(ctx, sub.topic, msg, handler)
		}
	}
}

func (a *adapter) invoke(ctx context.Context, topic string, msg eventbus.Message, handler eventbus.Handler) {
	hctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.handlerTimeout)
	defer cancel()

	defer func() {
		if r := recover(); r != nil && a.onPanic != nil {
			a.onPanic(topic, msg, r)
		}
	}()

	handler(hctx, msg)
}
