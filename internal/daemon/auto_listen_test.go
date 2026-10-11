package daemon

import (
	"testing"

	"ssh-tunnel-manager/internal/tunnel"
)

func TestTunnelViewShowsBoundProxyPortsWithoutLosingAutoConfig(t *testing.T) {
	cfg := tunnel.Config{Name: "proxy", Direction: tunnel.DirectionDynamic, LocalListen: "auto", HTTPListen: "auto"}
	status := tunnel.Status{Name: "proxy", State: "running", LocalListenAddress: "127.0.0.1:1081", HTTPListenAddress: "127.0.0.1:10801"}
	view := newTunnelView(cfg, true, status)
	if view.LocalListen != status.LocalListenAddress || view.HTTPListen != status.HTTPListenAddress {
		t.Fatalf("effective listeners = %q, %q", view.LocalListen, view.HTTPListen)
	}
	if view.ConfiguredLocalListen != "auto" || view.ConfiguredHTTPListen != "auto" {
		t.Fatalf("configured listeners = %q, %q", view.ConfiguredLocalListen, view.ConfiguredHTTPListen)
	}
	status.State = "stopped"
	view = newTunnelView(cfg, true, status)
	if view.LocalListen != "auto" || view.HTTPListen != "auto" {
		t.Fatalf("stopped listeners = %q, %q", view.LocalListen, view.HTTPListen)
	}
}
