package events

import (
	"sort"
	"strings"
)

const (
	StatusInfo           = "info"
	StatusFailed         = "failed"
	StatusFailedTests    = "failed-tests"
	StatusNeedsAuth      = "needs-auth"
	StatusBlocked        = "blocked"
	StatusStuck          = "stuck"
	StatusReadyForReview = "ready-for-review"
	StatusReadyForPR     = "ready-for-pr"
)

var knownStatuses = map[string]bool{
	StatusInfo:           true,
	StatusFailed:         true,
	StatusFailedTests:    true,
	StatusNeedsAuth:      true,
	StatusBlocked:        true,
	StatusStuck:          true,
	StatusReadyForReview: true,
	StatusReadyForPR:     true,
}

func Classify(event Event) string {
	return ClassifyFields(event.Type, event.Payload)
}

func ClassifyFields(eventType string, payload map[string]string) string {
	if payload != nil {
		if status := strings.ToLower(strings.TrimSpace(payload["status"])); knownStatuses[status] {
			return status
		}
	}
	text := strings.ToLower(eventType + " " + payloadText(payload))
	switch {
	case containsAny(text, "needs-auth", "auth required", "oauth", "login", "unauthorized", "forbidden", "401", "403", "permission denied"):
		return StatusNeedsAuth
	case containsAny(text, "failed-tests", "test failed", "tests failed", "failing test", "test failure") ||
		(strings.Contains(text, "test") && containsAny(text, "fail", "error", "exit 1")):
		return StatusFailedTests
	case containsAny(text, "ready-for-pr", "pr ready", "create pr", "open pr"):
		return StatusReadyForPR
	case containsAny(text, "ready-for-review", "review ready", "needs review"):
		return StatusReadyForReview
	case containsAny(text, "blocked", "waiting for input", "waiting for approval",
		"awaiting approval", "approval required", "needs approval", "permission prompt",
		"trust prompt", "waiting for login", "waiting for user"):
		return StatusBlocked
	case containsAny(text, "stuck", "timeout", "timed out", "deadlock", "unmet dependencies"):
		return StatusStuck
	case containsAny(text, "failed", "fail", "error", "exit 1", "non-zero"):
		return StatusFailed
	default:
		return StatusInfo
	}
}

func NeedsAttentionStatus(status string) bool {
	return status != "" && status != StatusInfo
}

func payloadText(payload map[string]string) string {
	if len(payload) == 0 {
		return ""
	}
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		if payload[key] == "" {
			continue
		}
		b.WriteByte(' ')
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(payload[key])
	}
	return b.String()
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
