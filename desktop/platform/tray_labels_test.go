package platform

import "testing"

func TestMenuLanguagesCoverSameActions(t *testing.T) {
	zh, en := MenuLabels("zh", "Portway"), MenuLabels("en", "Portway")
	if len(zh) != len(en) {
		t.Fatal("menu dictionaries differ")
	}
	for key, value := range zh {
		if value == "" || en[key] == "" {
			t.Fatalf("missing menu translation: %s", key)
		}
	}
	for key, want := range map[string]string{"about": "About Portway", "quit": "Quit Portway", "edit": "Edit", "window": "Window", "upload": "Upload", "download": "Download"} {
		if en[key] != want {
			t.Fatalf("%s: %q", key, en[key])
		}
	}
	if zh["about"] != "关于 Portway" || zh["edit"] != "编辑" {
		t.Fatal("Chinese native menu labels missing")
	}
	if zh["openMCP"] != "打开 MCP 设置" || en["openMCP"] != "Open MCP Settings" {
		t.Fatal("missing MCP menu translations")
	}
	if zh["startMCP"] != "启动 MCP" || zh["stopMCP"] != "停止 MCP" || en["startMCP"] != "Start MCP" || en["stopMCP"] != "Stop MCP" {
		t.Fatal("missing MCP toggle translations")
	}
	if zh["startTunnel"] != "启动线路" || zh["stopTunnel"] != "停止线路" || en["startTunnel"] != "Start Tunnel" || en["stopTunnel"] != "Stop Tunnel" || en["working"] != "Working..." {
		t.Fatal("missing tunnel control translations")
	}
}
