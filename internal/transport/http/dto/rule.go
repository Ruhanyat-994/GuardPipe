package dto

import (
	"time"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/advisory"
)

// --- rules catalogue — documentation/07-api-specification.md §8 ---

// RuleResponse matches `GET /rules/{id}`, and is what each item in
// `GET /rules`'s `data` array looks like too.
type RuleResponse struct {
	ID              string    `json:"id"`
	Engine          string    `json:"engine"`
	Category        string    `json:"category"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	Remediation     string    `json:"remediation"`
	DefaultSeverity string    `json:"default_severity"`
	CWE             []string  `json:"cwe"`
	OWASP           []string  `json:"owasp"`
	References      []string  `json:"references"`
	Tier            string    `json:"tier"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func FromRule(r advisory.Rule) RuleResponse {
	return RuleResponse{
		ID:              r.ID,
		Engine:          string(r.Engine),
		Category:        r.Category,
		Title:           r.Title,
		Description:     r.Description,
		Remediation:     r.Remediation,
		DefaultSeverity: string(r.DefaultSeverity),
		CWE:             emptyIfNil(r.CWE),
		OWASP:           emptyIfNil(r.OWASP),
		References:      emptyIfNil(r.References),
		Tier:            string(r.Tier),
		Enabled:         r.Enabled,
		CreatedAt:       r.CreatedAt,
		UpdatedAt:       r.UpdatedAt,
	}
}

func emptyIfNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// RuleListResponse matches `GET /rules`.
type RuleListResponse struct {
	Data       []RuleResponse `json:"data"`
	Pagination Pagination     `json:"pagination"`
}

// SetRuleEnabledRequest matches `PATCH /rules/{id}`. Enabled is a pointer
// so an omitted field is rejected by validation rather than silently
// read as false — this endpoint has exactly one job, and a caller that
// forgets the field should get a 400, not a rule disabled by accident.
type SetRuleEnabledRequest struct {
	Enabled *bool `json:"enabled" validate:"required"`
}
