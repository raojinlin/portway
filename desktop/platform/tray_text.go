package platform

import (
	"strings"
	"unicode/utf16"
)

// Windows menu text treats '&' as a mnemonic; names and error messages are data.
func windowsMenuText(text string) string {
	text = strings.Join(strings.Fields(strings.ReplaceAll(text, "\x00", " ")), " ")
	runes := []rune(text)
	if len(runes) > 120 {
		text = string(runes[:120]) + "\u2026"
	}
	return strings.ReplaceAll(text, "&", "&&")
}

func trayUTF16(text string, capacity int) []uint16 {
	if capacity <= 0 {
		return nil
	}
	value := utf16.Encode([]rune(strings.ReplaceAll(text, "\x00", " ")))
	if len(value) >= capacity {
		value = value[:capacity-1]
		if len(value) > 0 && value[len(value)-1] >= 0xD800 && value[len(value)-1] <= 0xDBFF {
			value = value[:len(value)-1]
		}
	}
	return append(value, 0)
}
