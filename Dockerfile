# syntax=docker/dockerfile:1

FROM golang:1.25-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/

ARG VERSION=dev
ARG COMMIT_SHA=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commitSHA=${COMMIT_SHA} -X main.buildTime=${BUILD_TIME}" \
    -o /out/guardpipe \
    ./cmd/guardpipe

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
WORKDIR /app

COPY --from=builder /out/guardpipe /app/guardpipe
COPY internal/store/migrations /app/internal/store/migrations

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/app/guardpipe"]
