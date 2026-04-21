package authropic

import (
	"context"
	"testing"

	"github.com/bestruirui/octopus/internal/transformer/model"
)

func TestTransformStream_ErrorEvent(t *testing.T) {
	o := &MessageOutbound{}

	// Anthropic stream error event: {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}
	eventData := []byte(`{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)

	result, err := o.TransformStream(context.Background(), eventData)

	if result != nil {
		t.Errorf("expected nil result for error event, got %v", result)
	}
	if err == nil {
		t.Fatal("expected error for error event, got nil")
	}

	respErr, ok := err.(*model.ResponseError)
	if !ok {
		t.Fatalf("expected ResponseError, got %T: %v", err, err)
	}
	if respErr.Detail.Type != "overloaded_error" {
		t.Errorf("error type: got %s, want overloaded_error", respErr.Detail.Type)
	}
	if respErr.Detail.Message != "Overloaded" {
		t.Errorf("error message: got %s, want Overloaded", respErr.Detail.Message)
	}
}

func TestTransformStream_ErrorEvent_RateLimit(t *testing.T) {
	o := &MessageOutbound{}

	eventData := []byte(`{"type":"error","error":{"type":"rate_limit_error","message":"Rate limit exceeded"}}`)

	result, err := o.TransformStream(context.Background(), eventData)

	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}

	respErr, ok := err.(*model.ResponseError)
	if !ok {
		t.Fatalf("expected ResponseError, got %T: %v", err, err)
	}
	if respErr.Detail.Type != "rate_limit_error" {
		t.Errorf("error type: got %s, want rate_limit_error", respErr.Detail.Type)
	}
}

func TestTransformStream_ErrorEvent_InvalidJSON(t *testing.T) {
	o := &MessageOutbound{}

	// type is "error" but can't parse the error detail
	eventData := []byte(`{"type":"error","malformed":"data"}`)

	result, err := o.TransformStream(context.Background(), eventData)

	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
	if err == nil {
		t.Fatal("expected error for malformed error event, got nil")
	}

	// Should fall through to the generic error message
	if !isResponseError(err) {
		// Not a ResponseError, just a generic fmt.Errorf
		if err.Error() == "" {
			t.Errorf("expected non-empty error message")
		}
	}
}

func TestTransformStream_PingEvent(t *testing.T) {
	o := &MessageOutbound{}

	eventData := []byte(`{"type":"ping"}`)
	result, err := o.TransformStream(context.Background(), eventData)

	if result != nil {
		t.Errorf("expected nil result for ping, got %v", result)
	}
	if err != nil {
		t.Errorf("expected nil error for ping, got %v", err)
	}
}

func TestTransformStream_ContentBlockStop(t *testing.T) {
	o := &MessageOutbound{}

	eventData := []byte(`{"type":"content_block_stop","index":0}`)
	result, err := o.TransformStream(context.Background(), eventData)

	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func isResponseError(err error) bool {
	_, ok := err.(*model.ResponseError)
	return ok
}