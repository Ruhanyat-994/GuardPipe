package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Ruhanyat-994/GuardPipe/internal/platform/config"
	"github.com/Ruhanyat-994/GuardPipe/internal/store/repo"
)

// runAdmin implements the `guardpipe admin grant-operator`/`revoke-operator`
// subcommands (BUILD_GUIDE.md Phase 14) — the *only* way a
// platform_operators row is ever created or removed. Deliberately not an
// HTTP endpoint: even a fully compromised operator session must never be
// able to mint a second operator through the API, so this needs direct
// database access (the same access level `guardpipe healthcheck`/the
// migration runner already assume), not a bearer token.
//
//	guardpipe admin grant-operator <email> --note "why"
//	guardpipe admin revoke-operator <email>
func runAdmin(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: guardpipe admin grant-operator <email> --note \"why\"")
		fmt.Fprintln(os.Stderr, "       guardpipe admin revoke-operator <email>")
		return 1
	}
	subcommand, email := args[0], args[1]

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "guardpipe admin: load config: "+err.Error())
		return 1
	}

	ctx := context.Background()
	db, err := repo.New(ctx, cfg.Data.DatabaseURL, 2)
	if err != nil {
		fmt.Fprintln(os.Stderr, "guardpipe admin: connect to database: "+err.Error())
		return 1
	}
	defer db.Close()

	users := repo.NewUserRepo(db.Pool)
	operators := repo.NewPlatformOperatorRepo(db.Pool)

	user, err := users.GetByEmail(ctx, email)
	if err != nil {
		fmt.Fprintln(os.Stderr, "guardpipe admin: no user with that email — they must register a normal account first: "+err.Error())
		return 1
	}

	switch subcommand {
	case "grant-operator":
		note := parseNoteFlag(args[2:])
		if note == "" {
			fmt.Fprintln(os.Stderr, "guardpipe admin: --note is required — record why this person needs platform-operator access")
			return 1
		}
		if _, err := operators.Grant(ctx, user.ID, nil, note); err != nil {
			fmt.Fprintln(os.Stderr, "guardpipe admin: grant operator: "+err.Error())
			return 1
		}
		fmt.Printf("granted platform-operator access to %s (%s)\n", email, user.ID)
		return 0

	case "revoke-operator":
		if err := operators.Revoke(ctx, user.ID); err != nil {
			if errors.Is(err, repo.ErrNotAnOperator) {
				fmt.Fprintln(os.Stderr, "guardpipe admin: "+email+" is not currently a platform operator")
				return 1
			}
			fmt.Fprintln(os.Stderr, "guardpipe admin: revoke operator: "+err.Error())
			return 1
		}
		fmt.Printf("revoked platform-operator access from %s (%s)\n", email, user.ID)
		return 0

	default:
		fmt.Fprintln(os.Stderr, "guardpipe admin: unknown subcommand "+subcommand+" — expected grant-operator or revoke-operator")
		return 1
	}
}

// parseNoteFlag reads a plain `--note "value"` (or `--note=value`) pair out
// of the remaining args — not a full flag parser, this command has exactly
// one optional flag.
func parseNoteFlag(args []string) string {
	for i, a := range args {
		if a == "--note" && i+1 < len(args) {
			return args[i+1]
		}
		const prefix = "--note="
		if len(a) > len(prefix) && a[:len(prefix)] == prefix {
			return a[len(prefix):]
		}
	}
	return ""
}
