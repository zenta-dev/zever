package ai

import (
	"testing"
)

func TestRole_consts(t *testing.T) {
	t.Parallel()
	cases := []struct {
		role Role
		want string
	}{
		{RoleSystem, "system"},
		{RoleUser, "user"},
		{RoleAssistant, "assistant"},
		{RoleTool, "tool"},
	}
	for _, tc := range cases {
		t.Run(string(tc.role), func(t *testing.T) {
			t.Parallel()
			if string(tc.role) != tc.want {
				t.Errorf("Role = %q, want %q", tc.role, tc.want)
			}
		})
	}
}

func TestMessage_zero(t *testing.T) {
	t.Parallel()
	var m Message
	if m.Role != "" {
		t.Errorf("zero Role = %q, want empty", m.Role)
	}
	if m.Content != "" {
		t.Errorf("zero Content = %q", m.Content)
	}
	if m.ToolCalls != nil {
		t.Errorf("zero ToolCalls = %v, want nil", m.ToolCalls)
	}
	if m.ToolCallID != "" {
		t.Errorf("zero ToolCallID = %q", m.ToolCallID)
	}
}

func TestMessage_withToolCalls(t *testing.T) {
	t.Parallel()
	m := Message{
		Role:       RoleAssistant,
		Content:    "hello",
		ToolCalls:  []ToolCall{{ID: "1", Name: "get_weather", Arguments: `{"city":"sf"}`}},
		ToolCallID: "",
	}
	if m.Role != RoleAssistant {
		t.Errorf("Role = %q, want assistant", m.Role)
	}
	if len(m.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(m.ToolCalls))
	}
	if m.ToolCalls[0].Name != "get_weather" {
		t.Errorf("ToolCall Name = %q", m.ToolCalls[0].Name)
	}
}

func TestTool_consts(t *testing.T) {
	t.Parallel()
	tool := Tool{Name: "search", Description: "search web", Parameters: map[string]any{"q": "string"}}
	if tool.Name != "search" {
		t.Errorf("Name = %q", tool.Name)
	}
	if tool.Description != "search web" {
		t.Errorf("Description = %q", tool.Description)
	}
	if tool.Parameters["q"] != "string" {
		t.Errorf("Parameters = %v", tool.Parameters)
	}
}

func TestToolChoice_consts(t *testing.T) {
	t.Parallel()
	if ToolChoiceAuto != "auto" {
		t.Errorf("ToolChoiceAuto = %q", ToolChoiceAuto)
	}
	if ToolChoiceRequired != "required" {
		t.Errorf("ToolChoiceRequired = %q", ToolChoiceRequired)
	}
	if ToolChoiceNone != "none" {
		t.Errorf("ToolChoiceNone = %q", ToolChoiceNone)
	}
}

func TestGenerateOptions_zero(t *testing.T) {
	t.Parallel()
	var opts GenerateOptions
	if opts.Temperature != nil {
		t.Errorf("Temperature = %v, want nil", opts.Temperature)
	}
	if opts.MaxTokens != 0 {
		t.Errorf("MaxTokens = %d, want 0", opts.MaxTokens)
	}
	if opts.TopP != nil {
		t.Errorf("TopP = %v, want nil", opts.TopP)
	}
	if opts.ResponseFormat != nil {
		t.Errorf("ResponseFormat = %v, want nil", opts.ResponseFormat)
	}
	if opts.Tools != nil {
		t.Errorf("Tools = %v, want nil", opts.Tools)
	}
	if opts.ParallelToolCalls != nil {
		t.Errorf("ParallelToolCalls = %v, want nil", opts.ParallelToolCalls)
	}
}

func TestGenerateOptions_withValues(t *testing.T) {
	t.Parallel()
	temp := float32(0.7)
	topP := float32(0.9)
	parallel := true
	opts := GenerateOptions{
		Temperature:       &temp,
		MaxTokens:         100,
		TopP:              &topP,
		ResponseFormat:    &ResponseFormat{Type: "json_object"},
		Tools:             []Tool{{Name: "tool1"}},
		ToolChoice:        ToolChoiceAuto,
		ParallelToolCalls: &parallel,
	}
	if *opts.Temperature != 0.7 {
		t.Errorf("Temperature = %v", *opts.Temperature)
	}
	if opts.MaxTokens != 100 {
		t.Errorf("MaxTokens = %d", opts.MaxTokens)
	}
	if *opts.TopP != 0.9 {
		t.Errorf("TopP = %v", *opts.TopP)
	}
	if opts.ResponseFormat.Type != "json_object" {
		t.Errorf("ResponseFormat.Type = %q", opts.ResponseFormat.Type)
	}
}

func TestGeneration_zero(t *testing.T) {
	t.Parallel()
	var g Generation
	if g.Content != "" {
		t.Errorf("Content = %q", g.Content)
	}
	if g.ToolCalls != nil {
		t.Errorf("ToolCalls = %v", g.ToolCalls)
	}
	if g.Usage.PromptTokens != 0 {
		t.Errorf("PromptTokens = %d", g.Usage.PromptTokens)
	}
	if g.FinishReason != "" {
		t.Errorf("FinishReason = %q", g.FinishReason)
	}
}

func TestUsage_values(t *testing.T) {
	t.Parallel()
	u := Usage{PromptTokens: 10, CompletionTokens: 20}
	if u.PromptTokens != 10 || u.CompletionTokens != 20 {
		t.Errorf("Usage = %+v", u)
	}
}

func TestEmbedOptions(t *testing.T) {
	t.Parallel()
	var o EmbedOptions
	if o.Dimensions != 0 {
		t.Errorf("Dimensions = %d, want 0", o.Dimensions)
	}
	o.Dimensions = 1536
	if o.Dimensions != 1536 {
		t.Errorf("Dimensions = %d", o.Dimensions)
	}
}

func TestStreamChunk_zero(t *testing.T) {
	t.Parallel()
	var c StreamChunk
	if c.Delta != "" {
		t.Errorf("Delta = %q", c.Delta)
	}
	if c.Done {
		t.Errorf("Done = true, want false")
	}
	if c.Usage != nil {
		t.Errorf("Usage = %v, want nil", c.Usage)
	}
	if c.Err != nil {
		t.Errorf("Err = %v, want nil", c.Err)
	}
}

func TestStreamChunk_withValues(t *testing.T) {
	t.Parallel()
	u := &Usage{PromptTokens: 5, CompletionTokens: 10}
	c := StreamChunk{
		Delta:          "hello",
		ReasoningDelta: "think",
		Done:           true,
		FinishReason:   "stop",
		Usage:          u,
		ToolCallID:     "call_1",
		ToolName:       "search",
		ToolArgsDelta:  `{"q":`,
	}
	if c.Delta != "hello" {
		t.Errorf("Delta = %q", c.Delta)
	}
	if c.ReasoningDelta != "think" {
		t.Errorf("ReasoningDelta = %q", c.ReasoningDelta)
	}
	if !c.Done {
		t.Errorf("Done = false, want true")
	}
	if c.Usage.PromptTokens != 5 {
		t.Errorf("Usage = %v", c.Usage)
	}
}
