package main

import (
	"net"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

var proxyHost = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

func trayProxy(listen string) (string, string) {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(listen))
	if err != nil {
		return "", ""
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return "", ""
	}
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	if host == "::" {
		host = "::1"
	}
	if net.ParseIP(host) == nil && !proxyHost.MatchString(host) {
		return "", ""
	}
	url := "socks5h://" + net.JoinHostPort(host, strconv.Itoa(port))
	if runtime.GOOS == "windows" {
		return url, "$env:ALL_PROXY='" + url + "'; $env:all_proxy=$env:ALL_PROXY; $env:http_proxy=$env:ALL_PROXY; $env:https_proxy=$env:ALL_PROXY"
	}
	return url, "export ALL_PROXY='" + url + "' all_proxy='" + url + "' http_proxy='" + url + "' https_proxy='" + url + "'"
}
