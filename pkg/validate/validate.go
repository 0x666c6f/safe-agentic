package validate

import (
	"fmt"
	"regexp"
	"strings"
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.\-]*$`)

func NameComponent(value, label string) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", label)
	}
	if !namePattern.MatchString(value) {
		return fmt.Errorf("%s contains invalid characters: %s. Allowed: letters, numbers, ., _, -", label, value)
	}
	return nil
}

func NetworkName(value string) error {
	if value == "none" {
		return nil
	}
	switch {
	case value == "bridge" || value == "host":
		return fmt.Errorf("unsafe network mode %q is not allowed. Create a dedicated Docker network and pass its name", value)
	case strings.HasPrefix(value, "container:"):
		return fmt.Errorf("unsafe network mode %q is not allowed. Create a dedicated Docker network and pass its name", value)
	}
	if value == "" {
		return fmt.Errorf("network name must not be empty")
	}
	if !namePattern.MatchString(value) {
		return fmt.Errorf("network name contains invalid characters: %s. Allowed: letters, numbers, ., _, -", value)
	}
	return nil
}

func PIDsLimit(value int) error {
	if value < 64 {
		return fmt.Errorf("PIDs limit must be >= 64 (got %d)", value)
	}
	return nil
}
