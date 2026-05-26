/**
 * @Project: Octopus
 * @File: gemini_relay_test.go
 * @Description: Gemini native route parsing regression tests for model-path content generation endpoints.
 * @Author: Ying Xinyao
 * @Contact: admin@loserrc.com | QQ: 1129414920
 * @Date: 2026-05-27
 * @Version: v1.0.0
 * @Copyright: (c) 2026 Ying Xinyao. All rights reserved.
 */

package handlers

import "testing"

func TestParseGeminiModelAction(t *testing.T) {
	tests := []struct {
		name       string
		action     string
		wantModel  string
		wantStream bool
		wantOK     bool
	}{
		{name: "generate", action: "/gemini-2.5-flash:generateContent", wantModel: "gemini-2.5-flash", wantOK: true},
		{name: "stream", action: "/gemini-2.5-flash:streamGenerateContent", wantModel: "gemini-2.5-flash", wantStream: true, wantOK: true},
		{name: "models prefix", action: "/models/gemini-2.5-flash:generateContent", wantModel: "gemini-2.5-flash", wantOK: true},
		{name: "unsupported", action: "/gemini-2.5-flash:countTokens", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modelName, stream, ok := parseGeminiModelAction(tt.action)
			if ok != tt.wantOK {
				t.Fatalf("ok = %t, want %t", ok, tt.wantOK)
			}
			if modelName != tt.wantModel {
				t.Fatalf("model = %q, want %q", modelName, tt.wantModel)
			}
			if stream != tt.wantStream {
				t.Fatalf("stream = %t, want %t", stream, tt.wantStream)
			}
		})
	}
}
