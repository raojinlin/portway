package tunnel

import (
	"net"
	"strconv"
	"testing"
)

func TestAutoListenSelectsFirstAvailablePort(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	port := occupied.Addr().(*net.TCPAddr).Port
	if port == 65535 {
		t.Skip("no higher port available")
	}
	selected, err := listenProxy("auto", port)
	if err != nil {
		t.Fatal(err)
	}
	defer selected.Close()
	actual := selected.Addr().(*net.TCPAddr)
	if actual.Port <= port || !actual.IP.Equal(net.IPv4(127, 0, 0, 1)) {
		t.Fatalf("auto listener = %s; occupied port = %d", actual, port)
	}
	if _, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(actual.Port))); err == nil {
		t.Fatal("selected port was not held open")
	}
}

func TestAutoListenMinimumsAndConfig(t *testing.T) {
	for _, first := range []int{1080, 10800} {
		ln, err := listenProxy("auto", first)
		if err != nil {
			t.Fatal(err)
		}
		if port := ln.Addr().(*net.TCPAddr).Port; port < first {
			t.Errorf("port %d is below %d", port, first)
		}
		ln.Close()
	}
	cfg := Config{Name: "proxy", Direction: DirectionDynamic, LocalListen: "auto", HTTPListen: "auto", SSHAddress: "host"}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatal(err)
	}
}
