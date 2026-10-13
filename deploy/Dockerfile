# syntax=docker/dockerfile:1

FROM golang:1.27-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY api ./api

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/trip-service ./cmd/trip-service

FROM gcr.io/distroless/static-debian12:nonroot

# distroless nonroot-образ уже запускает процесс от непривилегированного
# пользователя (uid 65532), явный USER не нужен и не изменил бы это.
COPY --from=builder /out/trip-service /trip-service

ENTRYPOINT ["/trip-service"]
