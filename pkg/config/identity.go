package config

import (
	"fmt"
	"os/exec"
	"strings"
)

// ParseIdentity parses an identity string of the form "Name <email>" and
// returns the name and email components. It mirrors the parse_identity()
// from the legacy shell implementation.
func ParseIdentity(identity string) (string, string, error) {
	if identity == "" {
		return "", "", fmt.Errorf("identity must not be empty")
	}
	ltIdx := strings.LastIndex(identity, "<")
	gtIdx := strings.LastIndex(identity, ">")
	if ltIdx < 0 || gtIdx < 0 || gtIdx < ltIdx {
		return "", "", fmt.Errorf("identity must be in format 'Name <email>': %s", identity)
	}
	name := strings.TrimSpace(identity[:ltIdx])
	email := identity[ltIdx+1 : gtIdx]
	if name == "" {
		return "", "", fmt.Errorf("name part is empty in identity: %s", identity)
	}
	if !strings.Contains(email, "@") || strings.HasPrefix(email, "@") || strings.HasSuffix(email, "@") {
		return "", "", fmt.Errorf("invalid email in identity: %s", email)
	}
	return name, email, nil
}

// DetectGitIdentity reads the global git config and returns "Name <email>" if
// both user.name and user.email are set, or an empty string otherwise. It
// ports the old shell detect_git_identity behavior.
func DetectGitIdentity() string {
	nameOut, err := exec.Command("git", "config", "--global", "user.name").Output()
	if err != nil {
		return ""
	}
	emailOut, err := exec.Command("git", "config", "--global", "user.email").Output()
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(string(nameOut))
	email := strings.TrimSpace(string(emailOut))
	if name == "" || email == "" {
		return ""
	}
	return fmt.Sprintf("%s <%s>", name, email)
}
