package repourl

import (
	"testing"
)

func TestClonePath(t *testing.T) {
	valid := []struct {
		input string
		want  string
	}{
		// HTTPS URLs
		{"https://github.com/org/repo", "org/repo"},
		{"https://github.com/org/repo.git", "org/repo"},
		{"https://gitlab.com/my-org/my-repo", "my-org/my-repo"},
		{"https://github.com/Org123/Repo.name", "Org123/Repo.name"},
		{"https://github.com/org/repo-with-dashes", "org/repo-with-dashes"},
		{"https://github.com/org/repo_underscores", "org/repo_underscores"},
		{"https://github.com/org/repo.with.dots", "org/repo.with.dots"},

		// SSH scp-style (git@)
		{"git@github.com:org/repo", "org/repo"},
		{"git@github.com:org/repo.git", "org/repo"},
		{"git@gitlab.com:my-org/my-repo", "my-org/my-repo"},
		{"git@github.com:Org123/Repo.name", "Org123/Repo.name"},

		// SSH URL-style (ssh://)
		{"ssh://git@github.com/org/repo", "org/repo"},
		{"ssh://git@github.com/org/repo.git", "org/repo"},
		{"ssh://github.com/org/repo", "org/repo"},

		// Dots and dashes in names
		{"https://github.com/my.org/my.repo", "my.org/my.repo"},
		{"https://github.com/org-1/repo-2", "org-1/repo-2"},
	}

	for _, tc := range valid {
		t.Run("valid/"+tc.input, func(t *testing.T) {
			got, err := ClonePath(tc.input)
			if err != nil {
				t.Errorf("ClonePath(%q) unexpected error: %v", tc.input, err)
				return
			}
			if got != tc.want {
				t.Errorf("ClonePath(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}

	invalid := []string{
		// no slash / no owner-repo separator
		"https://github.com/repo-only",
		"git@github.com:repo-only",

		// traversal attempts
		"https://github.com/../etc/passwd",
		"https://github.com/org/../etc",
		"git@github.com:../etc/passwd",

		// starts with dash (flag-like input)
		"-https://github.com/org/repo",
		"--option",

		// nested paths
		"https://github.com/org/repo/extra",
		"git@github.com:org/repo/extra",

		// empty owner or repo
		"https://github.com//repo",
		"https://github.com/org/",

		// dot-prefixed owner or repo
		"https://github.com/.hidden/repo",
		"https://github.com/org/.hidden",
		"git@github.com:.hidden/repo",
		"git@github.com:org/.hidden",

		// dash-prefixed owner or repo
		"https://github.com/-bad/repo",
		"https://github.com/org/-bad",
		"git@github.com:-bad/repo",
		"git@github.com:org/-bad",

		// special characters in names
		"https://github.com/org with space/repo",
		"https://github.com/org/repo name",
		"https://github.com/org@/repo",
		"https://github.com/org/repo!",

		// no scheme
		"github.com/org/repo",
		"org/repo",

		// empty string
		"",
	}

	for _, tc := range invalid {
		t.Run("invalid/"+tc, func(t *testing.T) {
			_, err := ClonePath(tc)
			if err == nil {
				t.Errorf("ClonePath(%q) expected error, got nil", tc)
			}
		})
	}
}

func TestUsesSSH(t *testing.T) {
	sshURLs := []string{
		"git@github.com:org/repo",
		"git@gitlab.com:org/repo.git",
		"ssh://github.com/org/repo",
		"ssh://git@github.com/org/repo",
	}
	for _, url := range sshURLs {
		t.Run("ssh/"+url, func(t *testing.T) {
			if !UsesSSH(url) {
				t.Errorf("UsesSSH(%q) = false, want true", url)
			}
		})
	}

	nonSSHURLs := []string{
		"https://github.com/org/repo",
		"https://gitlab.com/org/repo.git",
		"http://github.com/org/repo",
		"",
	}
	for _, url := range nonSSHURLs {
		t.Run("non-ssh/"+url, func(t *testing.T) {
			if UsesSSH(url) {
				t.Errorf("UsesSSH(%q) = true, want false", url)
			}
		})
	}
}

func TestDisplayLabel(t *testing.T) {
	tests := []struct {
		name  string
		repos []string
		want  string
	}{
		{
			name:  "empty",
			repos: []string{},
			want:  "",
		},
		{
			name:  "single repo",
			repos: []string{"https://github.com/org/repo"},
			want:  "org/repo",
		},
		{
			name:  "two repos",
			repos: []string{"https://github.com/org/repo1", "https://github.com/org/repo2"},
			want:  "org/repo1, org/repo2",
		},
		{
			name:  "three repos",
			repos: []string{"https://github.com/org/repo1", "https://github.com/org/repo2", "https://github.com/org/repo3"},
			want:  "org/repo1 + 2 more",
		},
		{
			name:  "four repos",
			repos: []string{"https://github.com/org/a", "https://github.com/org/b", "https://github.com/org/c", "https://github.com/org/d"},
			want:  "org/a + 3 more",
		},
		{
			name:  "single ssh repo",
			repos: []string{"git@github.com:org/repo"},
			want:  "org/repo",
		},
		{
			name:  "invalid url falls back to raw",
			repos: []string{"not-a-valid-url"},
			want:  "not-a-valid-url",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DisplayLabel(tc.repos)
			if got != tc.want {
				t.Errorf("DisplayLabel(%v) = %q, want %q", tc.repos, got, tc.want)
			}
		})
	}
}
