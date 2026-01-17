package helpers

import (
	"testing"

	"github.com/xiaot623/gogo/orchestrator/internal/repository"
)

func NewTestSQLiteStore(t *testing.T) *store.SQLiteStore {
	t.Helper()

	s, err := store.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
	}

	t.Cleanup(func() {
		_ = s.Close()
	})

	return s
}
