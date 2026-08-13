package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/gemini"
	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/config"
)

// runAIProbe is a throwaway manual-verification command for Phase 4
// (BUILD_GUIDE.md: "Done when: a throwaway CLI command can send a prompt
// through the fake in tests and through real Gemini manually"). It is not
// part of the product surface — no route, no test depends on it, nothing
// else calls it — just a way to point the real ai.Service at the real
// Gemini API by hand:
//
//	guardpipe aiprobe [evidence text...]
//
// It runs the explain_finding prompt against a small fixed rule/finding
// description plus whatever evidence text is given on the command line (a
// built-in SQL-injection snippet if none is given), then prints the
// model's response. Requires a real, valid GUARDPIPE_GEMINI_API_KEY(S) —
// this is the one place in the codebase allowed to make a live call.
func runAIProbe(args []string) int {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "guardpipe aiprobe: load config: "+err.Error())
		return 1
	}
	if !cfg.AI.Enabled {
		fmt.Fprintln(os.Stderr, "guardpipe aiprobe: GUARDPIPE_AI_ENABLED is false")
		return 1
	}

	client, err := gemini.NewClient("", nil, cfg.AI.KeyPool())
	if err != nil {
		fmt.Fprintln(os.Stderr, "guardpipe aiprobe: "+err.Error())
		return 1
	}
	svc := ai.NewService(client, ai.NewMemoryCache(), cfg.AI.CacheTTL, cfg.AI.ModelFast, cfg.AI.ModelSmart)

	evidence := strings.Join(args, " ")
	if evidence == "" {
		evidence = `cursor.execute("SELECT * FROM users WHERE id=" + user_id)`
	}

	result, err := svc.Run(context.Background(), ai.RunInput{
		PromptID: ai.PromptExplainFinding,
		Vars: map[string]string{
			"rule_id":  "codescan.injection.sql-string-concat",
			"title":    "SQL query built via string concatenation",
			"severity": "high",
			"cwe":      "CWE-89",
			"location": "aiprobe.py:1",
		},
		Untrusted: []ai.UntrustedBlock{{Label: "aiprobe.py:1", Content: evidence}},
	}, func(f domain.Finding) {
		fmt.Fprintln(os.Stderr, "guardpipe aiprobe: prompt_injection_attempt finding raised:", f.Title)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "guardpipe aiprobe: "+err.Error())
		return 1
	}
	if result.Discarded {
		fmt.Println("response discarded: suspected prompt injection (see stderr)")
		return 0
	}

	explained, ok := result.Value.(ai.ExplainFindingResponse)
	if !ok {
		fmt.Fprintf(os.Stderr, "guardpipe aiprobe: unexpected result type %T\n", result.Value)
		return 1
	}

	fmt.Printf(
		"provider:       gemini\nmodel:          %s\nfrom cache:     %v\ntokens in/out:  %d/%d\n\nwhat:           %s\nwhy_it_matters: %s\nhow_exploited:  %s\nconfidence:     %s\n",
		cfg.AI.ModelFast, result.FromCache, result.TokensIn, result.TokensOut,
		explained.What, explained.WhyItMatters, explained.HowExploited, explained.Confidence,
	)
	return 0
}
