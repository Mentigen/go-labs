package create

import (
	"context"

	"github.com/Mentigen/go-labs/internal/model"
)

//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

type usecase interface {
	Execute(ctx context.Context, input model.CreateTripInput) (model.CreateTripResult, error)
}
