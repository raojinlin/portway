package main

import "github.com/wailsapp/wails/v2/pkg/options/windows"

// Keep the native chrome palette aligned with web/src/theme.ts.
func applyThemeEvent(data []interface{}, apply func(bool, bool, uint8, uint8, uint8)) {
	if len(data) != 2 {
		return
	}
	mode, ok := data[0].(string)
	preference, preferenceOK := data[1].(string)
	if !ok || !preferenceOK || (preference != "system" && preference != mode) {
		return
	}
	followSystem := preference == "system"
	switch mode {
	case "light":
		apply(false, followSystem, 243, 245, 247)
	case "dark":
		apply(true, followSystem, 24, 26, 29)
	}
}

func desktopWindowsOptions() *windows.Options {
	light, dark := windows.RGB(243, 245, 247), windows.RGB(24, 26, 29)
	lightText, darkText := windows.RGB(36, 41, 47), windows.RGB(228, 231, 235)
	return &windows.Options{CustomTheme: &windows.ThemeSettings{
		LightModeTitleBar: light, LightModeTitleBarInactive: light,
		LightModeTitleText: lightText, LightModeTitleTextInactive: lightText,
		LightModeBorder: light, LightModeBorderInactive: light,
		DarkModeTitleBar: dark, DarkModeTitleBarInactive: dark,
		DarkModeTitleText: darkText, DarkModeTitleTextInactive: darkText,
		DarkModeBorder: dark, DarkModeBorderInactive: dark,
	}}
}
