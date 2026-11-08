package main

import (
	"net"
	"strconv"
	"testing"
)

func TestReserveDesktopBackendListenerFallsBackWhenPreferredPortIsBusy(t *testing.T) {
	occupied, err := net.Listen("tcp", net.JoinHostPort(desktopBackendHost, "0"))
	if err != nil {
		t.Fatalf("reserve occupied port: %v", err)
	}
	defer occupied.Close()

	_, occupiedPort, err := net.SplitHostPort(occupied.Addr().String())
	if err != nil {
		t.Fatalf("parse occupied port: %v", err)
	}
	if _, err := strconv.Atoi(occupiedPort); err != nil {
		t.Fatalf("occupied port is not numeric: %q", occupiedPort)
	}

	t.Setenv("HEPHAESTUS_DESKTOP_BACKEND_PORT", occupiedPort)

	listener, preferredPort, temporaryPort, fallbackError, err := reserveDesktopBackendListener()
	if err != nil {
		t.Fatalf("reserve desktop backend listener: %v", err)
	}
	defer listener.Close()

	if preferredPort != occupiedPort {
		t.Fatalf("preferred port = %q, want %q", preferredPort, occupiedPort)
	}
	if !temporaryPort {
		t.Fatalf("temporaryPort = false, want true")
	}
	if fallbackError == "" {
		t.Fatalf("fallbackError is empty")
	}

	_, actualPort, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("parse actual port: %v", err)
	}
	if actualPort == occupiedPort {
		t.Fatalf("actual port reused occupied port %q", occupiedPort)
	}
}

func TestNormalizeDesktopMCPListenPort(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty uses default later", input: "", want: ""},
		{name: "valid", input: " 9090 ", want: "9090"},
		{name: "normalizes leading zero", input: "08080", want: "8080"},
		{name: "zero invalid", input: "0", wantErr: true},
		{name: "too large invalid", input: "65536", wantErr: true},
		{name: "not numeric invalid", input: "abc", wantErr: true},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeDesktopMCPListenPort(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
