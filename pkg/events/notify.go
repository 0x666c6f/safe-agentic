package events

import "strings"

type NotifyTarget struct {
	Kind  string
	Value string
}

func ParseNotifyTargets(s string) []NotifyTarget {
	var targets []NotifyTarget
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if colonIdx := strings.Index(part, ":"); colonIdx > 0 {
			targets = append(targets, NotifyTarget{
				Kind:  part[:colonIdx],
				Value: part[colonIdx+1:],
			})
		} else {
			targets = append(targets, NotifyTarget{Kind: part})
		}
	}
	return targets
}
