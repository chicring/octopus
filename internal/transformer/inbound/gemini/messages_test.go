/**
 * @Project: Octopus
 * @File: messages_test.go
 * @Description: Gemini native inbound adapter regression tests for path-model request conversion and response shape.
 * @Author: Ying Xinyao
 * @Contact: admin@loserrc.com | QQ: 1129414920
 * @Date: 2026-05-27
 * @Version: v1.0.0
 * @Copyright: (c) 2026 Ying Xinyao. All rights reserved.
 */

package gemini

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bestruirui/octopus/internal/transformer/model"
)

func TestGeminiInboundTransformRequest(t *testing.T) {
	body := []byte(`{"contents":[{"role":"user","parts":[{"text":"Hello"}]}],"generationConfig":{"temperature":0.7,"topK":40,"maxOutputTokens":128}}`)
	inbound := &MessagesInbound{}

	req, err := inbound.TransformRequest(context.Background(), body)
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if len(req.Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(req.Messages))
	}
	if req.Messages[0].Role != "user" {
		t.Fatalf("role = %q, want user", req.Messages[0].Role)
	}
	if req.Messages[0].Content.Content == nil || *req.Messages[0].Content.Content != "Hello" {
		t.Fatalf("content = %#v, want Hello", req.Messages[0].Content.Content)
	}
	if req.TopK == nil || *req.TopK != 40 {
		t.Fatalf("TopK = %#v, want 40", req.TopK)
	}
	if req.MaxTokens == nil || *req.MaxTokens != 128 {
		t.Fatalf("MaxTokens = %#v, want 128", req.MaxTokens)
	}
}

func TestGeminiInboundConvertResponse(t *testing.T) {
	text := "pong"
	finish := "stop"
	inbound := &MessagesInbound{}

	body, err := inbound.ConvertResponseToClientFormat(context.Background(), &model.InternalLLMResponse{
		Choices: []model.Choice{{
			Index:        0,
			FinishReason: &finish,
			Message: &model.Message{
				Role:    "assistant",
				Content: model.MessageContent{Content: &text},
			},
		}},
	})
	if err != nil {
		t.Fatalf("ConvertResponseToClientFormat() error = %v", err)
	}

	var geminiResp model.GeminiGenerateContentResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(geminiResp.Candidates) != 1 || len(geminiResp.Candidates[0].Content.Parts) != 1 {
		t.Fatalf("unexpected candidates: %#v", geminiResp.Candidates)
	}
	if got := geminiResp.Candidates[0].Content.Parts[0].Text; got != "pong" {
		t.Fatalf("text = %q, want pong", got)
	}
	if geminiResp.Candidates[0].FinishReason == nil || *geminiResp.Candidates[0].FinishReason != "STOP" {
		t.Fatalf("finishReason = %#v, want STOP", geminiResp.Candidates[0].FinishReason)
	}
}
