package triphash_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	trip "github.com/Mentigen/go-labs/internal/domain/trip"
	"github.com/Mentigen/go-labs/internal/infra/triphash"
)

func TestParams(t *testing.T) {
	t.Parallel()
	base := trip.NewParams{UserID: uuid.MustParse("22222222-2222-2222-2222-222222222222"), DriverID: uuid.MustParse("33333333-3333-3333-3333-333333333333"), StartPoint: trip.Coordinates{Latitude: 55.75, Longitude: 37.61}, EndPoint: trip.Coordinates{Latitude: 55.8, Longitude: 37.7}, Price: 500}
	tests := []struct {
		name   string
		change func(*trip.NewParams)
	}{
		{"user", func(p *trip.NewParams) { p.UserID[0]++ }},
		{"driver", func(p *trip.NewParams) { p.DriverID[0]++ }},
		{"start latitude", func(p *trip.NewParams) { p.StartPoint.Latitude++ }},
		{"start longitude", func(p *trip.NewParams) { p.StartPoint.Longitude++ }},
		{"end latitude", func(p *trip.NewParams) { p.EndPoint.Latitude++ }},
		{"end longitude", func(p *trip.NewParams) { p.EndPoint.Longitude++ }},
		{"price", func(p *trip.NewParams) { p.Price++ }},
	}
	hasher := triphash.Hasher{}
	const legacy = "c805d58a4747dedbe1eda38ea5a66b4c26ccf138ac860023dceb10321a4ef36c"
	assert.Equal(t, legacy, hasher.Params(base))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			changed := base
			tt.change(&changed)
			assert.NotEqual(t, legacy, hasher.Params(changed))
		})
	}
}
