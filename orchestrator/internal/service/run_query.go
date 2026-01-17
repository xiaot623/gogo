package service

import (
	"context"
	"fmt"

	"github.com/xiaot623/gogo/orchestrator/internal/domain"
)

func (s *Service) GetRunEvents(ctx context.Context, runID string, afterTs int64, types []string, limit int) ([]domain.Event, error) {
	events, err := s.store.GetEvents(ctx, runID, afterTs, types, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get run events: %w", err)
	}
	return events, nil
}
