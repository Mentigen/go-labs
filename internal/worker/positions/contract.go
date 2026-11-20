package positions

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Mentigen/go-labs/internal/model"
)

//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}

type repository interface {
	StaleTrips(context.Context, time.Duration, time.Time, *model.PositionCursor, int) (model.StalePage, error)
	PositionRequestNeeded(context.Context, model.StaleTrip) (bool, error)
}
type gateway interface {
	RequestPosition(context.Context, uuid.UUID, uuid.UUID) error
}
type metrics interface {
	StaleCheck(context.Context, string)
	RowsRead(context.Context, int)
	RequestFinished(context.Context, string)
	RequestDuration(context.Context, string, time.Duration)
}
type logger interface {
	InfoContext(context.Context, string, ...any)
	WarnContext(context.Context, string, ...any)
}
type Ticker interface {
	C() <-chan time.Time
	Stop()
}
type Clock interface {
	Now() time.Time
	NewTicker(time.Duration) Ticker
}
