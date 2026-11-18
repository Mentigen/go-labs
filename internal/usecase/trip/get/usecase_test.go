package get_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"
	"github.com/Mentigen/go-labs/internal/usecase/trip/get"
)

func TestGetTrip(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	activeTrip := tripdomain.Trip{ID: id, Status: tripdomain.StatusActive}

	tests := []struct {
		name    string
		setup   func(*MocktripRepository)
		want    tripdomain.Trip
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "trip found",
			setup: func(repo *MocktripRepository) {
				repo.EXPECT().GetByID(gomock.Any(), id).Return(activeTrip, nil)
			},
			want:    activeTrip,
			wantErr: assert.NoError,
		},
		{
			name: "trip not found",
			setup: func(repo *MocktripRepository) {
				repo.EXPECT().GetByID(gomock.Any(), id).Return(tripdomain.Trip{}, tripdomain.ErrTripNotFound)
			},
			wantErr: func(t assert.TestingT, err error, _ ...any) bool {
				return assert.ErrorIs(t, err, tripdomain.ErrTripNotFound)
			},
		},
		{
			name: "repository unavailable",
			setup: func(repo *MocktripRepository) {
				repo.EXPECT().GetByID(gomock.Any(), id).Return(tripdomain.Trip{}, assert.AnError)
			},
			wantErr: func(t assert.TestingT, err error, _ ...any) bool {
				return assert.ErrorIs(t, err, assert.AnError)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMocktripRepository(gomock.NewController(t))
			tt.setup(repo)

			got, err := get.New(repo).Execute(t.Context(), id)
			tt.wantErr(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
