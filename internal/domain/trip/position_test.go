package trip_test

import (
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/Mentigen/go-labs/internal/domain/trip"
)

func TestNewPosition(t *testing.T) {
	lower := trip.Coordinates{Latitude: -90, Longitude: -180}
	upper := trip.Coordinates{Latitude: 90, Longitude: 180}
	yesterday := now.Add(-24 * time.Hour)
	limit := now.Add(time.Minute)
	badCoordinates := invalidField("coordinates")
	badTime := invalidField("recorded_at")
	tests := []struct {
		name    string
		id      uuid.UUID
		in      trip.PositionInput
		want    trip.Position
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "lower coordinate bounds", id: tripID,
			in:   trip.PositionInput{Coordinates: lower, RecordedAt: now},
			want: trip.Position{TripID: tripID, Coordinates: lower, RecordedAt: now}, wantErr: assert.NoError,
		},
		{
			name: "upper bounds and exactly one minute ahead", id: tripID,
			in:   trip.PositionInput{Coordinates: upper, RecordedAt: limit},
			want: trip.Position{TripID: tripID, Coordinates: upper, RecordedAt: limit}, wantErr: assert.NoError,
		},
		{
			name: "old point with a time zone offset", id: tripID,
			in:   trip.PositionInput{RecordedAt: yesterday.In(time.FixedZone("UTC+3", 3*60*60))},
			want: trip.Position{TripID: tripID, RecordedAt: yesterday}, wantErr: assert.NoError,
		},
		{
			name: "trip existence is checked by storage", id: uuid.Nil,
			in:   trip.PositionInput{RecordedAt: now},
			want: trip.Position{RecordedAt: now}, wantErr: assert.NoError,
		},
		{
			name: "latitude above 90", id: tripID,
			in: trip.PositionInput{Coordinates: trip.Coordinates{Latitude: 90.01}, RecordedAt: now}, wantErr: badCoordinates,
		},
		{
			name: "longitude below -180", id: tripID,
			in: trip.PositionInput{Coordinates: trip.Coordinates{Longitude: -180.01}, RecordedAt: now}, wantErr: badCoordinates,
		},
		{
			name: "NaN latitude", id: tripID,
			in: trip.PositionInput{Coordinates: trip.Coordinates{Latitude: math.NaN()}, RecordedAt: now}, wantErr: badCoordinates,
		},
		{
			name: "infinite longitude", id: tripID,
			in: trip.PositionInput{Coordinates: trip.Coordinates{Longitude: math.Inf(1)}, RecordedAt: now}, wantErr: badCoordinates,
		},
		{name: "missing timestamp", id: tripID, wantErr: badTime},
		{
			name: "more than one minute ahead", id: tripID,
			in: trip.PositionInput{RecordedAt: limit.Add(time.Nanosecond)}, wantErr: badTime,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := trip.NewPosition(tt.id, tt.in, now)
			tt.wantErr(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func invalidField(field string) assert.ErrorAssertionFunc {
	return func(t assert.TestingT, err error, _ ...any) bool {
		var validation *trip.ValidationError
		return assert.ErrorAs(t, err, &validation) && assert.Equal(t, field, validation.Field)
	}
}
