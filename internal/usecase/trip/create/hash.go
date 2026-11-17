package create

import tripdomain "github.com/Mentigen/go-labs/internal/domain/trip"

//go:generate go tool mockgen -source ${GOFILE} -destination hash_mocks_test.go -package ${GOPACKAGE}_test

type requestHasher interface {
	Params(params tripdomain.NewParams) string
}
