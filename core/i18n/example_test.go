package i18n_test

import (
	"context"
	"fmt"
	"testing/fstest"

	"github.com/zenta-dev/zever/i18n"
	"github.com/zenta-dev/zever/i18n/embed"
)

// ExampleOpen opens the embed adapter and translates a message.
func ExampleOpen() {
	if err := i18n.Register(i18n.Embed, embed.New); err != nil {
		return
	}

	fsys := fstest.MapFS{
		"en.json": {Data: []byte(`{"hello":"Hello, {{.name}}!"}`)},
	}

	be, err := i18n.Open(i18n.Embed, i18n.Options{
		Embed: i18n.EmbedOptions{FS: fsys, Fallback: "en"},
	})
	if err != nil {
		return
	}

	defer func() { _ = be.Close() }()

	s, err := be.Translate(context.Background(), "en", "hello", map[string]string{"name": "Ada"})
	if err != nil {
		return
	}

	fmt.Println(s)
	// Output: Hello, Ada!
}
