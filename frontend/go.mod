// This file exists only to mark frontend/ as a separate Go module boundary,
// so `go build ./...` and `go test ./...` from the repo root do not descend
// into frontend/node_modules (which can contain stray Go packages bundled
// inside npm dependencies). frontend/ has no Go code of its own.
module github.com/Ruhanyat-994/GuardPipe/frontend

go 1.23
