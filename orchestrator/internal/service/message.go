package service

import (
	"context"
	"fmt"

	"github.com/xiaot623/gogo/orchestrator/internal/domain"
)

func (s *Service) GetMessages(ctx context.Context, sessionID string, limit int, before string) ([]domain.Message, error) {
	messages, err := s.store.GetMessages(ctx, sessionID, limit, before)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}
	return messages, nil
}
