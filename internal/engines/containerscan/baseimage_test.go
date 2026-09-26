package containerscan

import "testing"

func TestFinalBaseImage(t *testing.T) {
	cases := []struct {
		name, dockerfile, want string
	}{
		// Resolves.
		{"single stage", "FROM node:20-alpine\nRUN npm ci\n", "node:20-alpine"},
		{"multi-stage takes the final stage", "FROM golang:1.26 AS build\nRUN go build\nFROM gcr.io/distroless/static-debian12:nonroot\n", "gcr.io/distroless/static-debian12:nonroot"},
		{"final stage built on an earlier alias", "FROM python:3.12-slim AS base\nFROM base AS app\nCOPY . .\n", "python:3.12-slim"},
		{"platform flag and lower-case keyword", "from --platform=linux/amd64 nginx:1.27-alpine as web\n", "nginx:1.27-alpine"},
		{"digest-pinned", "FROM alpine@sha256:0123abcd\n", "alpine@sha256:0123abcd"},
		// Must not resolve: nothing pullable without building.
		{"scratch", "FROM golang:1.26 AS build\nFROM scratch\nCOPY --from=build /app /app\n", ""},
		{"alias of scratch", "FROM scratch AS empty\nFROM empty\n", ""},
		{"ARG-based reference", "ARG GO_VERSION=1.26\nFROM golang:${GO_VERSION}\n", ""},
		{"no FROM at all", "RUN echo hi\n", ""},
		{"FROM only as a word in a comment or RUN", "# FROM evil:latest\nRUN echo FROM x\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := finalBaseImage(tc.dockerfile); got != tc.want {
				t.Errorf("finalBaseImage() = %q, want %q", got, tc.want)
			}
		})
	}
}
