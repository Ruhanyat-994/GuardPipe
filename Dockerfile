# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY api/ api/
COPY cmd/ cmd/
COPY internal/ internal/

ARG VERSION=dev
ARG COMMIT_SHA=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commitSHA=${COMMIT_SHA} -X main.buildTime=${BUILD_TIME}" \
    -o /out/guardpipe \
    ./cmd/guardpipe

# distroless has no shell, so the worker's clone workspace (GUARDPIPE_WORKSPACE_ROOT,
# default /var/lib/guardpipe/workspace) has to be pre-created and owned by the
# nonroot UID here, in the builder stage, then copied across with that ownership.
RUN mkdir -p /workspace && chown -R 65532:65532 /workspace

FROM gcr.io/distroless/static-debian12:nonroot AS runtime
WORKDIR /app

COPY --from=builder /out/guardpipe /app/guardpipe
COPY internal/store/migrations /app/internal/store/migrations
COPY --from=builder --chown=nonroot:nonroot /workspace /var/lib/guardpipe/workspace

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/app/guardpipe"]
