package container

import (
	"testing"

	"github.com/zenta-dev/zever/core/agent"
	"github.com/zenta-dev/zever/core/rag"
)

func TestAgentOption_applies(t *testing.T) {
	t.Parallel()

	var seen bool

	opts := agent.Options{}
	WithMaxParallel(4)(&opts)

	if opts.MaxParallel != 4 {
		t.Fatalf("MaxParallel = %d, want 4", opts.MaxParallel)
	}

	WithAgentObserver(func(agent.Event) { seen = true })(&opts)

	if opts.Observe == nil {
		t.Fatal("Observe not set")
	}

	opts.Observe(agent.Event{})

	if !seen {
		t.Fatal("observer never ran")
	}
}

func TestRAGOption_applies(t *testing.T) {
	t.Parallel()

	var cfg ragConfig
	WithTopK(7)(&cfg)
	WithSystemPrompt("sys")(&cfg)
	WithSearchIndex("idx")(&cfg)
	WithHybridSearch()(&cfg)
	WithRAGObserver(func(rag.Event) {})(&cfg)

	if cfg.options.TopK != 7 {
		t.Fatalf("TopK = %d, want 7", cfg.options.TopK)
	}

	if cfg.options.SystemPrompt != "sys" {
		t.Fatalf("SystemPrompt = %q", cfg.options.SystemPrompt)
	}

	if cfg.options.SearchIndex != "idx" {
		t.Fatalf("SearchIndex = %q", cfg.options.SearchIndex)
	}

	if !cfg.hybrid {
		t.Fatal("hybrid flag not set")
	}

	if cfg.options.Observe == nil {
		t.Fatal("Observe not set")
	}
}

func TestAgent_withOptions(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	l, err := c.Agent(WithMaxParallel(2))
	if err != nil {
		t.Fatalf("Agent() error = %v", err)
	}

	if l == nil {
		t.Fatal("Agent() returned nil loop")
	}
}

func TestRAG_withHybridSearch(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	e, err := c.RAG(WithHybridSearch())
	if err != nil {
		t.Fatalf("RAG() error = %v", err)
	}

	if e == nil {
		t.Fatal("RAG() returned nil engine")
	}
}
