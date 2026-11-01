package get_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"
	"github.com/Mentigen/go-labs/internal/usecase/trip/get"
)

var tripID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

func TestExecuteReturnsTrip(t *testing.T) {
	t.Parallel()

	repository := NewMocktripRepository(gomock.NewController(t))
	want := tripdomain.Trip{ID: tripID, Status: tripdomain.StatusActive}

	repository.EXPECT().GetByID(gomock.Any(), tripID).Return(want, nil)

	got, err := get.New(repository).Execute(context.Background(), tripID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got != want {
		t.Errorf("trip = %+v, want %+v", got, want)
	}
}

func TestExecuteTripNotFound(t *testing.T) {
	t.Parallel()

	repository := NewMocktripRepository(gomock.NewController(t))

	repository.EXPECT().GetByID(gomock.Any(), tripID).Return(tripdomain.Trip{}, tripdomain.ErrTripNotFound)

	_, err := get.New(repository).Execute(context.Background(), tripID)
	if !errors.Is(err, tripdomain.ErrTripNotFound) {
		t.Fatalf("error = %v, want ErrTripNotFound", err)
	}
}
