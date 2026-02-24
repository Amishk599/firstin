package ai

import "context"

// LLMProvider sends a prompt to an LLM and returns the raw text response.
// Used only by LLMJobAnalyzer; not exported to the rest of the system.
type LLMProvider interface {
	Complete(ctx context.Context, prompt string) (string, error)
}
