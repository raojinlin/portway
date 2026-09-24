//go:build !darwin && !windows

package platform

import (
	"os"
	"strings"
)

func SystemLanguage() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := os.Getenv(key); value != "" {
			if strings.HasPrefix(strings.ToLower(value), "zh") {
				return "zh"
			}
			return "en"
		}
	}
	return "en"
}

func Start(title string, actions TrayActions) error { return nil }
func Update(snapshot TraySnapshot)                  {}
func Stop()                                         {}
