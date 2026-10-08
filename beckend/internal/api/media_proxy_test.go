package api

import (
	"context"
	"errors"
	"net"
	"testing"
)

type staticMediaResolver []net.IPAddr

func (addresses staticMediaResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return addresses, nil
}

func TestPublicMediaIPRejectsSpecialUseNetworks(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		"0.1.2.3",
		"10.0.0.1",
		"100.64.0.1",
		"127.0.0.1",
		"169.254.1.1",
		"172.16.0.1",
		"192.0.2.1",
		"192.168.1.1",
		"198.18.0.1",
		"198.51.100.1",
		"203.0.113.1",
		"240.0.0.1",
		"::1",
		"64:ff9b::127.0.0.1",
		"fc00::1",
		"fe80::1",
		"2001:db8::1",
	} {
		if publicMediaIP(net.ParseIP(raw)) {
			t.Errorf("publicMediaIP(%s) = true, want false", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if !publicMediaIP(net.ParseIP(raw)) {
			t.Errorf("publicMediaIP(%s) = false, want true", raw)
		}
	}
}

func TestDialPublicMediaFallsBackAcrossPublicAddresses(t *testing.T) {
	resolver := staticMediaResolver{
		{IP: net.ParseIP("2606:4700:4700::1111")},
		{IP: net.ParseIP("1.1.1.1")},
	}
	attempts := 0
	connection, err := dialPublicMedia(context.Background(), "tcp", "cdn.example.test:443", resolver, func(_ context.Context, _, address string) (net.Conn, error) {
		attempts++
		if attempts == 1 {
			return nil, errors.New("IPv6 route unavailable")
		}
		client, server := net.Pipe()
		go server.Close()
		return client, nil
	})
	if err != nil {
		t.Fatalf("dialPublicMedia() error = %v", err)
	}
	defer connection.Close()
	if attempts != 2 {
		t.Fatalf("dial attempts = %d, want 2", attempts)
	}
}

func TestDialPublicMediaNeverDialsBlockedAddress(t *testing.T) {
	resolver := staticMediaResolver{{IP: net.ParseIP("100.64.0.1")}}
	dialed := false
	_, err := dialPublicMedia(context.Background(), "tcp", "cdn.example.test:443", resolver, func(context.Context, string, string) (net.Conn, error) {
		dialed = true
		return nil, errors.New("unexpected")
	})
	if err == nil || dialed {
		t.Fatalf("error=%v dialed=%t", err, dialed)
	}
}
