package main

import "testing"

func TestThemeEvent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		data   []interface{}
		want   [4]uint8
		system bool
	}{
		{"light", []interface{}{"light", "light"}, [4]uint8{0, 243, 245, 247}, false},
		{"dark", []interface{}{"dark", "dark"}, [4]uint8{1, 24, 26, 29}, false},
		{"system light", []interface{}{"light", "system"}, [4]uint8{0, 243, 245, 247}, true},
		{"system dark", []interface{}{"dark", "system"}, [4]uint8{1, 24, 26, 29}, true},
		{"missing", nil, [4]uint8{}, false},
		{"unknown", []interface{}{"system", "system"}, [4]uint8{}, false},
		{"object", []interface{}{map[string]interface{}{"mode": "dark"}, "system"}, [4]uint8{}, false},
		{"mismatch", []interface{}{"dark", "light"}, [4]uint8{}, false},
		{"invalid preference", []interface{}{"dark", nil}, [4]uint8{}, false},
		{"extra", []interface{}{"dark", "system", "extra"}, [4]uint8{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got [4]uint8
			var system bool
			applyThemeEvent(tc.data, func(dark, followSystem bool, r, g, b uint8) {
				system = followSystem
				got = [4]uint8{0, r, g, b}
				if dark {
					got[0] = 1
				}
			})
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			if system != tc.system {
				t.Fatalf("follow system = %v, want %v", system, tc.system)
			}
		})
	}
}
