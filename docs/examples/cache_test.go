package examples_test

import (
	"context"
	"fmt"
	"time"

	cachememory "github.com/zenta-dev/zever/adapters/cache/memory"
	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/shared/codec"
)

// spaceCard is the struct stored through the typed cache helper.
type spaceCard struct {
	Title      string `json:"title"`
	PriceCents int64  `json:"price_cents"`
}

// ExampleCache_openGetSet mirrors the cache doc sample: open the memory
// backend, set a raw value with a TTL, read it back, then round-trip a
// struct through the typed helper.
func Example_cacheOpenGetSet() {
	ctx := context.Background()
	backend, err := cachememory.New(cache.Options{})
	if err != nil {
		fmt.Println("open error")
		return
	}
	defer func() { _ = backend.Close(ctx) }()

	if err := backend.Set(ctx, "space:42", []byte("loft"), 5*time.Minute); err != nil {
		fmt.Println("set error")
		return
	}
	raw, err := backend.Get(ctx, "space:42")
	if err != nil {
		fmt.Println("get error")
		return
	}
	fmt.Println(string(raw))

	typed := cache.NewTyped[string, spaceCard](backend, codec.JSONCodec[spaceCard]{})
	if err := typed.Set(ctx, "space:card:42", spaceCard{Title: "Loft", PriceCents: 12000}, 5*time.Minute); err != nil {
		fmt.Println("typed set error")
		return
	}
	card, err := typed.Get(ctx, "space:card:42")
	if err != nil {
		fmt.Println("typed get error")
		return
	}
	fmt.Println(card.Title, card.PriceCents)
	// Output:
	// loft
	// Loft 12000
}
