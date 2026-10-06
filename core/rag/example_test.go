package rag_test

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/core/ai"
	"github.com/zenta-dev/zever/core/rag"
	"github.com/zenta-dev/zever/core/vectorstore"
)

type exampleAI struct{}

func (exampleAI) Generate(_ context.Context, _ string, _ []ai.Message, _ ai.GenerateOptions) (ai.Generation, error) {
	return ai.Generation{Content: "grounded"}, nil
}

func (exampleAI) Stream(context.Context, string, []ai.Message, ai.GenerateOptions) (<-chan ai.StreamChunk, error) {
	ch := make(chan ai.StreamChunk)
	close(ch)
	return ch, nil
}

func (exampleAI) Embed(_ context.Context, _ string, inputs []string, _ ai.EmbedOptions) ([][]float32, error) {
	out := make([][]float32, len(inputs))
	for i := range inputs {
		out[i] = []float32{1, 0}
	}
	return out, nil
}

func (exampleAI) Close() error { return nil }

type exampleStore struct{}

func (exampleStore) Upsert(context.Context, vectorstore.Vector) error { return nil }

func (exampleStore) UpsertBatch(context.Context, []vectorstore.Vector) error { return nil }

func (exampleStore) Delete(context.Context, string) error { return nil }

func (exampleStore) Query(context.Context, []float32, int) ([]vectorstore.ScoreMatch, error) {
	return nil, nil
}

func (exampleStore) Close() error { return nil }

// ExampleEngine ingests a document and answers a question.
func ExampleEngine() {
	engine, err := rag.New(exampleAI{}, exampleStore{}, rag.Options{Model: "model-x"})
	if err != nil {
		fmt.Println("new error")
		return
	}

	if err := engine.Ingest(context.Background(), []rag.Document{{ID: "d1", Content: "zever compiles schemas"}}); err != nil {
		fmt.Println("ingest error")
		return
	}

	ans, err := engine.Answer(context.Background(), "what does zever do?", 0)
	if err != nil {
		fmt.Println("answer error")
		return
	}

	fmt.Println(ans.Text)
	// Output: grounded
}
