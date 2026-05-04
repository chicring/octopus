package builtin

import (
	"sync"
	"testing"

	"github.com/bestruirui/octopus/internal/provider"
	"github.com/bestruirui/octopus/internal/transformer/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
)

func TestRegisterAndGet(t *testing.T) {
	tests := []struct {
		id         provider.ProviderID
		legacyType outbound.OutboundType
	}{
		{"openai-chat", outbound.OutboundTypeOpenAIChat},
		{"openai-response", outbound.OutboundTypeOpenAIResponse},
		{"anthropic", outbound.OutboundTypeAnthropic},
		{"gemini", outbound.OutboundTypeGemini},
		{"volcengine", outbound.OutboundTypeVolcengine},
		{"openai-embedding", outbound.OutboundTypeOpenAIEmbedding},
	}

	for _, tt := range tests {
		p := provider.Get(tt.id)
		if p == nil {
			t.Errorf("Get(%q) returned nil", tt.id)
			continue
		}
		if p.ID() != tt.id {
			t.Errorf("ID() = %q, want %q", p.ID(), tt.id)
		}
		lt := p.LegacyType()
		if lt == nil {
			t.Errorf("LegacyType() is nil for %q", tt.id)
			continue
		}
		if *lt != tt.legacyType {
			t.Errorf("LegacyType() = %d, want %d for %q", *lt, tt.legacyType, tt.id)
		}
	}
}

func TestGetByLegacyType(t *testing.T) {
	tests := []struct {
		legacyType outbound.OutboundType
		wantID     provider.ProviderID
	}{
		{outbound.OutboundTypeOpenAIChat, "openai-chat"},
		{outbound.OutboundTypeAnthropic, "anthropic"},
		{outbound.OutboundTypeGemini, "gemini"},
		{outbound.OutboundTypeVolcengine, "volcengine"},
		{outbound.OutboundTypeOpenAIEmbedding, "openai-embedding"},
	}

	for _, tt := range tests {
		p := provider.GetByLegacyType(tt.legacyType)
		if p == nil {
			t.Errorf("GetByLegacyType(%d) returned nil", tt.legacyType)
			continue
		}
		if p.ID() != tt.wantID {
			t.Errorf("ID() = %q, want %q", p.ID(), tt.wantID)
		}
	}
}

func TestList(t *testing.T) {
	providers := provider.List()
	if len(providers) != 7 {
		t.Errorf("List() returned %d providers, want 7", len(providers))
	}
}

func TestGetOutbound(t *testing.T) {
	p := provider.GetOutbound("openai-chat")
	if p == nil {
		t.Error(`GetOutbound("openai-chat") returned nil`)
	}
	p = provider.GetOutbound("nonexistent")
	if p != nil {
		t.Error(`GetOutbound("nonexistent") should return nil`)
	}
}

func TestResolveProviderIDFromType(t *testing.T) {
	tests := []struct {
		legacyType outbound.OutboundType
		want       provider.ProviderID
	}{
		{outbound.OutboundTypeOpenAIChat, "openai-chat"},
		{outbound.OutboundTypeAnthropic, "anthropic"},
		{99, ""},
	}

	for _, tt := range tests {
		got := provider.ResolveProviderIDFromType(tt.legacyType)
		if got != tt.want {
			t.Errorf("ResolveProviderIDFromType(%d) = %q, want %q", tt.legacyType, got, tt.want)
		}
	}
}

func TestIsChatProvider(t *testing.T) {
	if !provider.IsChatProvider("openai-chat") {
		t.Error("openai-chat should support chat")
	}
	if provider.IsChatProvider("openai-embedding") {
		t.Error("openai-embedding should not support chat")
	}
	if provider.IsChatProvider("nonexistent") {
		t.Error("nonexistent should not support chat")
	}
}

func TestIsEmbeddingProvider(t *testing.T) {
	if !provider.IsEmbeddingProvider("openai-embedding") {
		t.Error("openai-embedding should support embedding")
	}
	if provider.IsEmbeddingProvider("openai-chat") {
		t.Error("openai-chat should not support embedding")
	}
}

func TestGetCredentialSchema(t *testing.T) {
	schema := provider.GetCredentialSchema("openai-chat")
	if schema == nil {
		t.Fatal("openai-chat should have credential schema")
	}
	if schema.AuthType != provider.AuthTypeManual {
		t.Errorf("AuthType = %q, want %q", schema.AuthType, provider.AuthTypeManual)
	}
	if len(schema.Fields) != 2 {
		t.Errorf("Fields count = %d, want 2", len(schema.Fields))
	}
}

func TestGetAuthFlow(t *testing.T) {
	flow := provider.GetAuthFlow("openai-chat")
	if flow == nil {
		t.Error("openai-chat should have auth flow")
	}
	if flow.Type() != provider.AuthTypeManual {
		t.Errorf("AuthFlow Type() = %q, want %q", flow.Type(), provider.AuthTypeManual)
	}
}

func TestGetModelFetcher(t *testing.T) {
	if provider.GetModelFetcher("openai-chat") == nil {
		t.Error("openai-chat should have model fetcher")
	}
	if provider.GetModelFetcher("anthropic") == nil {
		t.Error("anthropic should have model fetcher")
	}
	if provider.GetModelFetcher("gemini") == nil {
		t.Error("gemini should have model fetcher")
	}
}

func TestNormalizeChannelType(t *testing.T) {
	t.Run("BothEmpty", func(t *testing.T) {
		pid, lt, err := provider.NormalizeChannelType(nil, nil)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if pid != "" || lt != 0 {
			t.Errorf("got pid=%q lt=%d, want empty/0", pid, lt)
		}
	})

	t.Run("OnlyProviderID", func(t *testing.T) {
		pid := "openai-chat"
		gotPID, gotLT, err := provider.NormalizeChannelType(&pid, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotPID != "openai-chat" {
			t.Errorf("pid = %q, want %q", gotPID, "openai-chat")
		}
		if gotLT != outbound.OutboundTypeOpenAIChat {
			t.Errorf("lt = %d, want %d", gotLT, outbound.OutboundTypeOpenAIChat)
		}
	})

	t.Run("OnlyLegacyType", func(t *testing.T) {
		lt := outbound.OutboundTypeAnthropic
		gotPID, gotLT, err := provider.NormalizeChannelType(nil, &lt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotPID != "anthropic" {
			t.Errorf("pid = %q, want %q", gotPID, "anthropic")
		}
		if gotLT != outbound.OutboundTypeAnthropic {
			t.Errorf("lt = %d, want %d", gotLT, outbound.OutboundTypeAnthropic)
		}
	})

	t.Run("BothConsistent", func(t *testing.T) {
		pid := "anthropic"
		lt := outbound.OutboundTypeAnthropic
		gotPID, gotLT, err := provider.NormalizeChannelType(&pid, &lt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotPID != "anthropic" || gotLT != outbound.OutboundTypeAnthropic {
			t.Errorf("got pid=%q lt=%d", gotPID, gotLT)
		}
	})

	t.Run("BothInconsistent", func(t *testing.T) {
		pid := "anthropic"
		lt := outbound.OutboundTypeOpenAIChat
		_, _, err := provider.NormalizeChannelType(&pid, &lt)
		if err != provider.ErrTypeMismatch {
			t.Errorf("err = %v, want ErrTypeMismatch", err)
		}
	})

	t.Run("UnknownProviderID", func(t *testing.T) {
		pid := "nonexistent"
		_, _, err := provider.NormalizeChannelType(&pid, nil)
		if err == nil {
			t.Error("expected error for unknown provider_id")
		}
	})

	t.Run("EmptyProviderIDWithLegacy", func(t *testing.T) {
		pid := ""
		lt := outbound.OutboundTypeGemini
		gotPID, gotLT, err := provider.NormalizeChannelType(&pid, &lt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotPID != "gemini" {
			t.Errorf("pid = %q, want %q", gotPID, "gemini")
		}
		if gotLT != outbound.OutboundTypeGemini {
			t.Errorf("lt = %d, want %d", gotLT, outbound.OutboundTypeGemini)
		}
	})
}

func TestOutboundFactory(t *testing.T) {
	tests := []struct {
		id provider.ProviderID
	}{
		{"openai-chat"},
		{"openai-response"},
		{"openai-embedding"},
		{"anthropic"},
		{"gemini"},
		{"volcengine"},
	}

	for _, tt := range tests {
		t.Run(string(tt.id), func(t *testing.T) {
			p := provider.Get(tt.id)
			if p == nil {
				t.Fatalf("Get(%q) returned nil", tt.id)
			}
			factory := p.OutboundFactory()
			if factory == nil {
				t.Fatalf("OutboundFactory() returned nil for %q", tt.id)
			}
			ob := factory()
			if ob == nil {
				t.Errorf("factory() returned nil for %q", tt.id)
			}
		})
	}
}

func TestGetOutboundByLegacyType(t *testing.T) {
	tests := []struct {
		legacyType outbound.OutboundType
	}{
		{outbound.OutboundTypeOpenAIChat},
		{outbound.OutboundTypeOpenAIResponse},
		{outbound.OutboundTypeAnthropic},
		{outbound.OutboundTypeGemini},
		{outbound.OutboundTypeVolcengine},
		{outbound.OutboundTypeOpenAIEmbedding},
	}

	for _, tt := range tests {
		o := provider.GetOutboundByLegacyType(tt.legacyType)
		if o == nil {
			t.Errorf("GetOutboundByLegacyType(%d) returned nil", tt.legacyType)
		}
	}

	o := provider.GetOutboundByLegacyType(99)
	if o != nil {
		t.Error("GetOutboundByLegacyType(99) should return nil")
	}
}

func TestCapabilities(t *testing.T) {
	tests := []struct {
		id            provider.ProviderID
		wantChat      bool
		wantEmbedding bool
	}{
		{"openai-chat", true, false},
		{"openai-response", true, false},
		{"openai-embedding", false, true},
		{"anthropic", true, false},
		{"gemini", true, false},
		{"volcengine", true, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.id), func(t *testing.T) {
			p := provider.Get(tt.id)
			if p == nil {
				t.Fatalf("Get(%q) returned nil", tt.id)
			}
			caps := p.Capabilities()
			if caps.Chat != tt.wantChat {
				t.Errorf("Chat = %v, want %v for %q", caps.Chat, tt.wantChat, tt.id)
			}
			if caps.Embedding != tt.wantEmbedding {
				t.Errorf("Embedding = %v, want %v for %q", caps.Embedding, tt.wantEmbedding, tt.id)
			}
		})
	}
}

func TestOptionalInterfaces(t *testing.T) {
	ids := []provider.ProviderID{
		"openai-chat", "openai-response", "openai-embedding",
		"anthropic", "gemini", "volcengine",
	}

	for _, id := range ids {
		t.Run(string(id), func(t *testing.T) {
			p := provider.Get(id)
			if p == nil {
				t.Fatalf("Get(%q) returned nil", id)
			}

			if _, ok := p.(provider.SchemaProvider); !ok {
				t.Errorf("%q does not implement SchemaProvider", id)
			}
			if _, ok := p.(provider.AuthProvider); !ok {
				t.Errorf("%q does not implement AuthProvider", id)
			}
			if _, ok := p.(provider.ModelFetchProvider); !ok {
				t.Errorf("%q does not implement ModelFetchProvider", id)
			}
		})
	}
}

func TestDisplayName(t *testing.T) {
	names := map[provider.ProviderID]string{
		"openai-chat":      "OpenAI Chat",
		"openai-response":  "OpenAI Response",
		"openai-embedding": "OpenAI Embedding",
		"anthropic":        "Anthropic",
		"gemini":           "Gemini",
		"volcengine":       "Volcengine",
	}

	for id, want := range names {
		p := provider.Get(id)
		if p == nil {
			t.Errorf("Get(%q) returned nil", id)
			continue
		}
		if p.DisplayName() != want {
			t.Errorf("DisplayName() = %q, want %q for %q", p.DisplayName(), want, id)
		}
	}
}

func TestCredentialSchemaFields(t *testing.T) {
	ids := []provider.ProviderID{
		"openai-chat", "anthropic", "gemini", "volcengine",
	}

	for _, id := range ids {
		t.Run(string(id), func(t *testing.T) {
			schema := provider.GetCredentialSchema(id)
			if schema == nil {
				t.Fatalf("GetCredentialSchema(%q) returned nil", id)
			}
			if schema.AuthType != provider.AuthTypeManual {
				t.Errorf("AuthType = %q, want %q", schema.AuthType, provider.AuthTypeManual)
			}
			if len(schema.Fields) != 2 {
				t.Fatalf("Fields count = %d, want 2", len(schema.Fields))
			}
			if schema.Fields[0].Key != "base_url" {
				t.Errorf("Fields[0].Key = %q, want %q", schema.Fields[0].Key, "base_url")
			}
			if schema.Fields[1].Key != "api_key" {
				t.Errorf("Fields[1].Key = %q, want %q", schema.Fields[1].Key, "api_key")
			}
			if !schema.Fields[1].Secret {
				t.Error("api_key field should be secret")
			}
		})
	}
}

func TestGetNonexistent(t *testing.T) {
	if provider.Get("nonexistent") != nil {
		t.Error("Get(nonexistent) should return nil")
	}
	if provider.GetCredentialSchema("nonexistent") != nil {
		t.Error("GetCredentialSchema(nonexistent) should return nil")
	}
	if provider.GetAuthFlow("nonexistent") != nil {
		t.Error("GetAuthFlow(nonexistent) should return nil")
	}
	if provider.GetModelFetcher("nonexistent") != nil {
		t.Error("GetModelFetcher(nonexistent) should return nil")
	}
}

func TestConcurrentRegistryAccess(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(4)
		go func() { defer wg.Done(); provider.List() }()
		go func() { defer wg.Done(); provider.Get("openai-chat") }()
		go func() { defer wg.Done(); provider.GetByLegacyType(outbound.OutboundTypeOpenAIChat) }()
		go func() { defer wg.Done(); provider.GetOutbound("anthropic") }()
	}
	wg.Wait()
}

func TestGetOutboundFactoryReturnsNonNil(t *testing.T) {
	p := provider.Get("openai-chat")
	if p == nil {
		t.Fatal("Get(openai-chat) returned nil")
	}
	factory := p.OutboundFactory()
	o1 := factory()
	o2 := factory()
	if o1 == nil || o2 == nil {
		t.Fatal("factory() returned nil")
	}
}

func TestNormalizeChannelType_UnknownLegacyType(t *testing.T) {
	lt := outbound.OutboundType(99)
	gotPID, gotLT, err := provider.NormalizeChannelType(nil, &lt)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if gotPID != "" {
		t.Errorf("pid = %q, want empty for unknown legacy type", gotPID)
	}
	if gotLT != 99 {
		t.Errorf("lt = %d, want 99", gotLT)
	}
}

func TestNormalizeChannelType_EmptyProviderIDOnly(t *testing.T) {
	pid := ""
	gotPID, gotLT, err := provider.NormalizeChannelType(&pid, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if gotPID != "" || gotLT != 0 {
		t.Errorf("got pid=%q lt=%d, want empty/0", gotPID, gotLT)
	}
}

func TestOutboundImplementsInterface(t *testing.T) {
	ids := []provider.ProviderID{
		"openai-chat", "openai-response", "openai-embedding",
		"anthropic", "gemini", "volcengine",
	}

	for _, id := range ids {
		t.Run(string(id), func(t *testing.T) {
			o := provider.GetOutbound(id)
			if o == nil {
				t.Fatalf("GetOutbound(%q) returned nil", id)
			}
			var _ model.Outbound = o
		})
	}
}
