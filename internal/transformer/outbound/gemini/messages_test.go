/**
 * @Project: Octopus
 * @File: messages_test.go
 * @Description: Gemini outbound regression tests for native passthrough body preservation and path model routing.
 * @Author: Ying Xinyao
 * @Contact: admin@loserrc.com | QQ: 1129414920
 * @Date: 2026-05-27
 * @Version: v1.0.0
 * @Copyright: (c) 2026 Ying Xinyao. All rights reserved.
 */

package gemini

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/bestruirui/octopus/internal/transformer/model"
)

func TestGeminiOutboundPassthroughPreservesBody(t *testing.T) {
	raw := []byte(`{"contents":[{"role":"user","parts":[{"text":"Hello"}]}]}`)
	outbound := &MessagesOutbound{}

	req, err := outbound.TransformRequest(context.Background(), &model.InternalLLMRequest{
		Model:        "gemini-2.5-flash",
		RawRequest:   raw,
		RawAPIFormat: model.APIFormatGeminiContents,
	}, "https://generativelanguage.googleapis.com/v1beta", "upstream-key")
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != string(raw) {
		t.Fatalf("body changed:\ngot  %s\nwant %s", body, raw)
	}
	if strings.Contains(string(body), `"model"`) {
		t.Fatalf("Gemini native body must not contain OpenAI-style model field: %s", body)
	}
	if got := req.URL.Path; got != "/v1beta/models/gemini-2.5-flash:generateContent" {
		t.Fatalf("path = %q", got)
	}
	if got := req.URL.Query().Get("key"); got != "upstream-key" {
		t.Fatalf("key query = %q", got)
	}
}
