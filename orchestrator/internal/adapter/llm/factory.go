package llm

import (
	"log"
	"os"
	"time"
)

const (
	// EnvGogoMode is the environment variable name for mode selection.
	EnvGogoMode = "GOGO_MODE"
	// ModeMock indicates mock mode should be used.
	ModeMock = "MOCK"
)

// NewLLMClient creates an LLM client based on the GOGO_MODE environment variable.
// If GOGO_MODE=MOCK, returns a MockClient; otherwise returns a real Client.
func NewLLMClient(baseURL, apiKey string, timeout time.Duration) LLMClient {
	mode := os.Getenv(EnvGogoMode)

	if mode == ModeMock {
		log.Println("GOGO_MODE=MOCK detected, using mock LLM client")
		return NewMockClient()
	}

	return NewClient(baseURL, apiKey, timeout)
}
