package repourl

import (
	"fmt"
	"regexp"
	"strings"
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._\-]*$`)

func ClonePath(repoURL string) (string, error) {
	if strings.HasPrefix(repoURL, "-") {
		return "", fmt.Errorf("invalid repo URL: %s", repoURL)
	}
	path, err := cloneURLPath(repoURL)
	if err != nil {
		return "", err
	}
	owner, repo, err := splitClonePath(repoURL, path)
	if err != nil {
		return "", err
	}
	if err := validateCloneOwner(owner); err != nil {
		return "", err
	}
	if err := validateCloneRepo(repo); err != nil {
		return "", err
	}
	return owner + "/" + repo, nil
}

func cloneURLPath(repoURL string) (string, error) {
	path := strings.TrimSuffix(repoURL, ".git")
	switch {
	case strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "ssh://"):
		return clonePathFromSchemeURL(repoURL, path)
	case strings.Contains(path, ":") && strings.Contains(path, "/"):
		return clonePathFromSCP(path), nil
	default:
		return "", fmt.Errorf("invalid repo URL: %s (must be https://, ssh://, or git@host:owner/repo)", repoURL)
	}
}

func clonePathFromSchemeURL(repoURL, path string) (string, error) {
	idx := strings.Index(path, "://")
	path = path[idx+3:]
	slashIdx := strings.Index(path, "/")
	if slashIdx < 0 {
		return "", fmt.Errorf("invalid repo URL: %s", repoURL)
	}
	return path[slashIdx+1:], nil
}

func clonePathFromSCP(path string) string {
	colonIdx := strings.LastIndex(path, ":")
	return path[colonIdx+1:]
}

func splitClonePath(repoURL, path string) (string, string, error) {
	if !strings.Contains(path, "/") {
		return "", "", fmt.Errorf("invalid repo URL: %s (no owner/repo)", repoURL)
	}
	owner, repo, ok := strings.Cut(path, "/")
	if !ok {
		return "", "", fmt.Errorf("invalid repo URL: %s (no owner/repo)", repoURL)
	}
	if strings.Contains(repo, "/") {
		return "", "", fmt.Errorf("invalid repo URL: %s (nested path)", repoURL)
	}
	return owner, repo, nil
}

func validateCloneOwner(owner string) error {
	if owner == "" || strings.HasPrefix(owner, ".") || strings.HasPrefix(owner, "-") {
		return fmt.Errorf("invalid repo owner: %q", owner)
	}
	if !namePattern.MatchString(owner) {
		return fmt.Errorf("invalid repo owner: %q", owner)
	}
	return nil
}

func validateCloneRepo(repo string) error {
	if repo == "" || strings.HasPrefix(repo, ".") || strings.HasPrefix(repo, "-") {
		return fmt.Errorf("invalid repo name: %q", repo)
	}
	if !namePattern.MatchString(repo) {
		return fmt.Errorf("invalid repo name: %q", repo)
	}
	return nil
}

func UsesSSH(url string) bool {
	return strings.HasPrefix(url, "git@") || strings.HasPrefix(url, "ssh://")
}

func DisplayLabel(repos []string) string {
	if len(repos) == 0 {
		return ""
	}
	slugs := make([]string, 0, len(repos))
	for _, r := range repos {
		s, err := ClonePath(r)
		if err != nil {
			slugs = append(slugs, r)
		} else {
			slugs = append(slugs, s)
		}
	}
	if len(slugs) <= 2 {
		return strings.Join(slugs, ", ")
	}
	return fmt.Sprintf("%s + %d more", slugs[0], len(slugs)-1)
}
