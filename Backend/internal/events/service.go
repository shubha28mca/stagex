package events

import (
	"context"

	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// Service holds event business logic. Today it is a thin pass-through to the
// repository, but keeping the layer means future rules (e.g. hiding events with
// a pending hall booking) have an obvious home.
type Service struct {
	repo Repository
}

// NewService builds an events Service.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// List returns events matching the filter.
func (s *Service) List(ctx context.Context, f Filter) ([]Event, error) {
	list, err := s.repo.List(ctx, f)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []Event{}
	}
	return list, nil
}

// Get returns a single event with its categories, or a 404 error.
func (s *Service) Get(ctx context.Context, id string) (*Event, error) {
	e, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, httpx.ErrNotFound("event not found")
	}
	cats, err := s.repo.ListCategories(ctx, id)
	if err != nil {
		return nil, err
	}
	e.Categories = cats
	return e, nil
}
