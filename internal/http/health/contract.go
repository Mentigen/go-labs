package health

import "context"

//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test

type pinger interface {
	Ping(ctx context.Context) error
}
