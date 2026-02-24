package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func makeTestServer(t *testing.T, statusCode int, body any) (*httptest.Server, *http.Client) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, srv.Client()
}

func TestComplete_Success(t *testing.T) {
	resp := chatResponse{
		Choices: []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}{
			{Message: struct {
				Content string `json:"content"`
			}{Content: `{"role_type":"backend"}`}},
		},
	}
	srv, client := makeTestServer(t, http.StatusOK, resp)

	provider := NewOpenAIProvider(srv.URL, "test-key", "test-model", client)
	got, err := provider.Complete(context.Background(), "analyze this")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `{"role_type":"backend"}` {
		t.Errorf("got %q, want json string", got)
	}
}

func TestComplete_HTTPError(t *testing.T) {
	srv, client := makeTestServer(t, http.StatusInternalServerError, map[string]string{"error": "server error"})

	provider := NewOpenAIProvider(srv.URL, "test-key", "test-model", client)
	_, err := provider.Complete(context.Background(), "analyze this")
	if err == nil {
		t.Fatal("expected error on 5xx response")
	}
}

func TestComplete_RateLimited(t *testing.T) {
	srv, client := makeTestServer(t, http.StatusTooManyRequests, map[string]string{"error": "rate limited"})

	provider := NewOpenAIProvider(srv.URL, "test-key", "test-model", client)
	_, err := provider.Complete(context.Background(), "analyze this")
	if err == nil {
		t.Fatal("expected error on 429 response")
	}
}

func TestComplete_EmptyChoices(t *testing.T) {
	resp := chatResponse{Choices: nil}
	srv, client := makeTestServer(t, http.StatusOK, resp)

	provider := NewOpenAIProvider(srv.URL, "test-key", "test-model", client)
	_, err := provider.Complete(context.Background(), "analyze this")
	if err == nil {
		t.Fatal("expected error when LLM returns no choices")
	}
}

func TestComplete_SetsAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		resp := chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{Message: struct {
					Content string `json:"content"`
				}{Content: "ok"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	provider := NewOpenAIProvider(srv.URL, "my-secret-key", "test-model", srv.Client())
	_, _ = provider.Complete(context.Background(), "hello")

	if gotAuth != "Bearer my-secret-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer my-secret-key")
	}
}

func TestComplete_SendsStructuredOutputFormat(t *testing.T) {
	var gotReq chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		resp := chatResponse{
			Choices: []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			}{
				{Message: struct {
					Content string `json:"content"`
				}{Content: "{}"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	provider := NewOpenAIProvider(srv.URL, "key", "gpt-4o-mini", srv.Client())
	_, _ = provider.Complete(context.Background(), "analyze this")

	if gotReq.ResponseFormat.Type != "json_schema" {
		t.Errorf("response_format.type = %q, want json_schema", gotReq.ResponseFormat.Type)
	}
	if gotReq.ResponseFormat.JSONSchema.Name != "job_insights" {
		t.Errorf("response_format.json_schema.name = %q, want job_insights", gotReq.ResponseFormat.JSONSchema.Name)
	}
	if gotReq.Temperature != 0 {
		t.Errorf("temperature = %d, want 0", gotReq.Temperature)
	}
}
