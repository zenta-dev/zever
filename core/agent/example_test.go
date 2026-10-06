package agent_test

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/core/agent"
	"github.com/zenta-dev/zever/core/ai"
)

type exampleAI struct{}

func (exampleAI) Generate(_ context.Context, _ string, _ []ai.Message, _ ai.GenerateOptions) (ai.Generation, error) {
	return ai.Generation{Content: "hello from agent"}, nil
}

func (exampleAI) Stream(context.Context, string, []ai.Message, ai.GenerateOptions) (<-chan ai.StreamChunk, error) {
	ch := make(chan ai.StreamChunk)
	close(ch)
	return ch, nil
}

func (exampleAI) Embed(context.Context, string, []string, ai.EmbedOptions) ([][]float32, error) {
	return nil, nil
}

func (exampleAI) Close() error { return nil }

// ExampleNew builds a Loop and runs a single turn.
func ExampleNew() {
	loop, err := agent.New(exampleAI{}, agent.Options{Model: "model-x"})
	if err != nil {
		fmt.Println("new error")
		return
	}

	res, err := loop.Run(context.Background(), []ai.Message{{Role: ai.RoleUser, Content: "hi"}})
	if err != nil {
		fmt.Println("run error")
		return
	}

	fmt.Println(res.Content)
	// Output: hello from agent
}
