package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Mentigen/go-labs/internal/model"
	"github.com/Mentigen/go-labs/internal/repository"
	"github.com/Mentigen/go-labs/internal/txmanager"
)

type CreateTripInput struct {
	UserID   uuid.UUID
	DriverID uuid.UUID

	StartPoint model.Coordinates
	EndPoint   model.Coordinates

	Price int64
}

type TripService struct {
	repository *repository.TripRepository
	txManager  txmanager.TxManager
}

func NewTripService(repository *repository.TripRepository, txManager txmanager.TxManager) *TripService {
	return &TripService{
		repository: repository,
		txManager:  txManager,
	}
}

func newTrip(input CreateTripInput) model.Trip {
	now := time.Now().UTC()

	return model.Trip{
		ID:       uuid.New(),
		UserID:   input.UserID,
		DriverID: input.DriverID,

		StartPoint: input.StartPoint,
		EndPoint:   input.EndPoint,

		Price:  input.Price,
		Status: model.TripStatusActive,

		StartedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (s *TripService) Create(ctx context.Context, input CreateTripInput) (model.Trip, error) {
	trip := newTrip(input)

	err := s.txManager.Do(ctx, func(ctx context.Context) error {
		if err := s.repository.Create(ctx, trip); err != nil {
			return err
		}

		return s.repository.AddCreatedHistory(ctx, trip.ID)
	})
	if err != nil {
		return model.Trip{}, err
	}

	return trip, nil
}

func (s *TripService) GetByID(ctx context.Context, tripID uuid.UUID) (model.Trip, error) {
	return s.repository.GetByID(ctx, tripID)
}

func (s *TripService) Finish(ctx context.Context, tripID uuid.UUID) (model.Trip, error) {
	finishedAt := time.Now().UTC()

	var trip model.Trip

	err := s.txManager.Do(ctx, func(ctx context.Context) error {
		if err := s.repository.Finish(ctx, tripID, finishedAt); err != nil {
			return err
		}

		if err := s.repository.AddCompletedHistory(ctx, tripID); err != nil {
			return err
		}

		found, err := s.repository.GetByID(ctx, tripID)
		if err != nil {
			return err
		}

		trip = found

		return nil
	})
	if err != nil {
		return model.Trip{}, err
	}

	return trip, nil
}
