package policy

import (
	"fmt"
	"regexp"
	"strings"

	"cyberagent-workbench/internal/tools"
)

type Decision struct {
	Allowed       bool   `json:"allowed"`
	Reason        string `json:"reason"`
	Risk          string `json:"risk,omitempty"`
	NeedsApproval bool   `json:"needs_approval,omitempty"`
}

type Checker interface {
	CheckText(context string, text string) Decision
	CheckToolCall(call tools.Call) Decision
}

type DefaultChecker struct {
	denyPatterns []*regexp.Regexp
}

func NewDefaultChecker() DefaultChecker {
	phrases := []string{
		`(?i)\bmasscan\b`,
		`(?i)\bhydra\b`,
		`(?i)\bmeterpreter\b`,
		`(?i)\bmsfconsole\b`,
		`(?i)\bbackdoor\b`,
		`(?i)\bcredential\s+(dump|theft|steal|exfil)`,
		`(?i)\bsteal\s+(cookie|token|password|credential)`,
		`(?i)\bdisable\s+(defender|antivirus|edr)`,
		`(?i)0\.0\.0\.0/0`,
		`(?i)\b(public|internet)\s+(scan|attack|exploit)`,
		`(?i)rm\s+-rf\s+(/|\*)`,
	}
	patterns := make([]*regexp.Regexp, 0, len(phrases))
	for _, phrase := range phrases {
		patterns = append(patterns, regexp.MustCompile(phrase))
	}
	return DefaultChecker{denyPatterns: patterns}
}

func (c DefaultChecker) CheckText(context string, text string) Decision {
	for _, pattern := range c.denyPatterns {
		if pattern.MatchString(text) {
			return Decision{
				Allowed: false,
				Reason:  fmt.Sprintf("blocked %s because it matched safety pattern %q", context, pattern.String()),
				Risk:    "high",
			}
		}
	}
	return Decision{Allowed: true, Reason: "allowed by default cyber safety policy"}
}

func (c DefaultChecker) CheckToolCall(call tools.Call) Decision {
	var parts []string
	parts = append(parts, call.Name)
	for k, v := range call.Args {
		parts = append(parts, k, v)
	}
	joined := strings.Join(parts, " ")
	decision := c.CheckText("tool_call", joined)
	if !decision.Allowed {
		return decision
	}
	if strings.Contains(strings.ToLower(joined), "nmap") {
		return Decision{
			Allowed:       true,
			Reason:        "network scan commands require explicit scoped approval",
			Risk:          "medium",
			NeedsApproval: true,
		}
	}
	return Decision{Allowed: true, Reason: "tool call allowed"}
}
