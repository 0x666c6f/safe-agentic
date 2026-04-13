package validate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.\-]*$`)
var memoryPattern = regexp.MustCompile(`^[1-9][0-9]*[bkmgBKMG]$`)

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

func MemoryLimit(value string) error {
	if value == "" {
		return nil
	}
	if !memoryPattern.MatchString(value) {
		return fmt.Errorf("memory limit must look like 512m, 8g, etc. (got %q)", value)
	}
	return nil
}

func CPUs(value string) error {
	if value == "" {
		return nil
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || n <= 0 {
		return fmt.Errorf("cpus must be a positive number (got %q)", value)
	}
	return nil
}
