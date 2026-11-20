package list_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/Mentigen/go-labs/internal/domain/trip"
	"github.com/Mentigen/go-labs/internal/usecase/position/list"
)

func TestExecute(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	route := []trip.Position{{ID: 1, TripID: id}, {ID: 2, TripID: id}}
	tests := []struct {
		name    string
		want    []trip.Position
		wantErr error
	}{
		{name: "trip has no points", want: []trip.Position{}},
		{name: "return points in route order", want: route},
		{name: "trip not found", wantErr: trip.ErrTripNotFound},
		{name: "cannot read positions", wantErr: assert.AnError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockrepository(gomock.NewController(t))
			repo.EXPECT().ListPositions(gomock.Any(), id).Return(tt.want, tt.wantErr)

			got, err := list.New(repo).Execute(t.Context(), id)
			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.want, got)
		})
	}
}
