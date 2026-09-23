package platform

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestWindowsMenuText(t *testing.T) {
	if got := windowsMenuText("a&b\n\tline\x00name"); got != "a&&b line name" {
		t.Fatalf("unsafe label: %q", got)
	}
	if got := windowsMenuText(strings.Repeat("界", 121)); len([]rune(got)) != 121 || !strings.HasSuffix(got, "\u2026") {
		t.Fatal("long name not bounded safely")
	}
}

func TestTrayUTF16(t *testing.T) {
	for _, test := range []struct {
		text     string
		capacity int
		want     string
	}{
		{"abc", 4, "abc"}, {"abc", 3, "ab"}, {"abc", 1, ""},
		{"a\U0001F680z", 3, "a"}, {"a\U0001F680z", 4, "a\U0001F680"},
		{"a\x00b", 10, "a b"}, {"", 1, ""},
	} {
		encoded := trayUTF16(test.text, test.capacity)
		if len(encoded) > test.capacity || encoded[len(encoded)-1] != 0 {
			t.Fatal("invalid notification buffer length or terminator")
		}
		if got := string(utf16.Decode(encoded[:len(encoded)-1])); got != test.want {
			t.Fatalf("got %q want %q", got, test.want)
		}
	}
	if !reflect.DeepEqual(trayUTF16("text", 0), []uint16(nil)) {
		t.Fatal("zero capacity must return nil")
	}
}
