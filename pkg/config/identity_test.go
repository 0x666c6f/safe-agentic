package config

import (
	"testing"
)

func TestParseIdentity(t *testing.T) {
	t.Run("valid identities", func(t *testing.T) {
		tests := []struct {
			input     string
			wantName  string
			wantEmail string
		}{
			{
				input:     "Alice Bob <alice@example.com>",
				wantName:  "Alice Bob",
				wantEmail: "alice@example.com",
			},
			{
				input:     "Agent Bot <bot@example.com>",
				wantName:  "Agent Bot",
				wantEmail: "bot@example.com",
			},
			{
				input:     "  Florian  <florian@morpho.org>",
				wantName:  "Florian",
				wantEmail: "florian@morpho.org",
			},
			{
				input:     "A <a@b.com>",
				wantName:  "A",
				wantEmail: "a@b.com",
			},
		}
		for _, tt := range tests {
			name, email, err := ParseIdentity(tt.input)
			if err != nil {
				t.Errorf("ParseIdentity(%q) unexpected error: %v", tt.input, err)
				continue
			}
			if name != tt.wantName {
				t.Errorf("ParseIdentity(%q) name = %q, want %q", tt.input, name, tt.wantName)
			}
			if email != tt.wantEmail {
				t.Errorf("ParseIdentity(%q) email = %q, want %q", tt.input, email, tt.wantEmail)
			}
		}
	})

	t.Run("invalid identities", func(t *testing.T) {
		invalid := []string{
			"",
			"Alice Bob",
			"Alice Bob <",
			"<email@example.com>",
			"Alice <notanemail>",
			"Alice <@example.com>",
			"Alice <alice@>",
			" <alice@example.com>",
		}
		for _, input := range invalid {
			_, _, err := ParseIdentity(input)
			if err == nil {
				t.Errorf("ParseIdentity(%q) expected error, got nil", input)
			}
		}
	})
}
