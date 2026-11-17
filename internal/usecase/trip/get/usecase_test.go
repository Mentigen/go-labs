package get_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"
	"github.com/Mentigen/go-labs/internal/usecase/trip/get"
)

var tripID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

func TestExecute(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		want    tripdomain.Trip
		wantErr error
	}{
		{name: "found", want: tripdomain.Trip{ID: tripID, Status: tripdomain.StatusActive}},
		{name: "not found", wantErr: tripdomain.ErrTripNotFound},
		{name: "storage failure", wantErr: errors.New("storage unavailable")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repository := NewMocktripRepository(gomock.NewController(t))
			repository.EXPECT().GetByID(gomock.Any(), tripID).Return(tt.want, tt.wantErr)
			got, err := get.New(repository).Execute(context.Background(), tripID)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
