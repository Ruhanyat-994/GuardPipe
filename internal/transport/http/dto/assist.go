package dto

import "github.com/Ruhanyat-994/GuardPipe/internal/modules/assist"

// AssistRequest is `POST /findings/{id}/assist`.
type AssistRequest struct {
	Action string `json:"action" validate:"required,oneof=explain remediate fix"`
}

// AssistExplain is the "Explain" answer.
type AssistExplain struct {
	What         string `json:"what"`
	WhyItMatters string `json:"why_it_matters"`
	HowExploited string `json:"how_exploited"`
	Confidence   string `json:"confidence"`
}

// AssistStep is one remediation step.
type AssistStep struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Code   string `json:"code,omitempty"`
}

// AssistRemediate is the "Remediation" answer.
type AssistRemediate struct {
	Summary      string       `json:"summary"`
	Steps        []AssistStep `json:"steps"`
	Verification string       `json:"verification"`
	Confidence   string       `json:"confidence"`
}

// AssistFix is the "Fix it" answer.
type AssistFix struct {
	Patch       string   `json:"patch"`
	Explanation string   `json:"explanation"`
	Confidence  string   `json:"confidence"`
	Caveats     []string `json:"caveats"`
}

// AssistResponse is one command's answer; exactly one of explain/remediate/fix is set.
type AssistResponse struct {
	Action        string           `json:"action"`
	TokensCharged int64            `json:"tokens_charged"`
	AlreadyPaid   bool             `json:"already_paid"`
	SourceUsed    bool             `json:"source_used"`
	Explain       *AssistExplain   `json:"explain,omitempty"`
	Remediate     *AssistRemediate `json:"remediate,omitempty"`
	Fix           *AssistFix       `json:"fix,omitempty"`
}

func FromAssistReply(r *assist.Reply) AssistResponse {
	out := AssistResponse{Action: string(r.Action), TokensCharged: r.TokensCharged, AlreadyPaid: r.AlreadyPaid, SourceUsed: r.SourceUsed}
	if e := r.Explain; e != nil {
		out.Explain = &AssistExplain{What: e.What, WhyItMatters: e.WhyItMatters, HowExploited: e.HowExploited, Confidence: e.Confidence}
	}
	if m := r.Remediate; m != nil {
		steps := make([]AssistStep, len(m.Steps))
		for i, st := range m.Steps {
			steps[i] = AssistStep{Title: st.Title, Detail: st.Detail, Code: st.Code}
		}
		out.Remediate = &AssistRemediate{Summary: m.Summary, Steps: steps, Verification: m.Verification, Confidence: m.Confidence}
	}
	if f := r.Fix; f != nil {
		caveats := f.Caveats
		if caveats == nil {
			caveats = []string{}
		}
		out.Fix = &AssistFix{Patch: f.Patch, Explanation: f.Explanation, Confidence: f.Confidence, Caveats: caveats}
	}
	return out
}
