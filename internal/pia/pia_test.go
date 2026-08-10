package pia

import (
	"strings"
	"testing"
)

const sampleWgOutput = `interface: pia
  public key: ABC123def456GHI789jkl012MNO345pqr678STU901vwx
  private key: (hidden)
  listening port: 51820

peer: DEF456ghi789JKL012mno345PQR678stu901VWX234abc
  preshared key: (hidden)
  endpoint: 123.45.67.89:51820
  allowed ips: 0.0.0.0/0
  latest handshake: 1 minute, 23 seconds ago
  transfer: 841.18 MiB received, 124.78 MiB sent
`

func TestParseWgOutput(t *testing.T) {
	s := parseWgOutput(sampleWgOutput)

	if !s.Connected {
		t.Error("expected connected=true")
	}
	if want := "ABC123def456GHI789jkl012MNO345pqr678STU901vwx"; s.PublicKey != want {
		t.Errorf("public key = %q, want %q", s.PublicKey, want)
	}
	if want := "123.45.67.89"; s.Endpoint != want {
		t.Errorf("endpoint = %q (want port stripped: %q)", s.Endpoint, want)
	}
	if want := "1 minute, 23 seconds ago"; s.HandshakeAgo != want {
		t.Errorf("handshake ago = %q, want %q", s.HandshakeAgo, want)
	}
	if want := 83; s.HandshakeSecs != want {
		t.Errorf("handshake secs = %d, want %d", s.HandshakeSecs, want)
	}
	rxFloat := 841.18 * 1024 * 1024
	rxWant := int64(rxFloat)
	if s.TransferRx != rxWant {
		t.Errorf("transfer rx = %d, want %d", s.TransferRx, rxWant)
	}
	txFloat := 124.78 * 1024 * 1024
	txWant := int64(txFloat)
	if s.TransferTx != txWant {
		t.Errorf("transfer tx = %d, want %d", s.TransferTx, txWant)
	}
	if s.TransferRxStr != "841.18 MiB received" {
		t.Errorf("transfer rx str = %q", s.TransferRxStr)
	}
}

func TestParseWgOutputHandshakeNoComma(t *testing.T) {
	// wg prints bare seconds ("x seconds ago") when the handshake is fresh.
	out := strings.Replace(sampleWgOutput, "latest handshake: 1 minute, 23 seconds ago", "latest handshake: 9 seconds ago", 1)
	s := parseWgOutput(out)
	if want := 9; s.HandshakeSecs != want {
		t.Errorf("handshake secs = %d, want %d", s.HandshakeSecs, want)
	}
}

func TestParseWgOutputHours(t *testing.T) {
	out := strings.Replace(sampleWgOutput, "latest handshake: 1 minute, 23 seconds ago", "latest handshake: 2 hours, 5 minutes ago", 1)
	s := parseWgOutput(out)
	if want := 2*3600 + 5*60; s.HandshakeSecs != want {
		t.Errorf("handshake secs = %d, want %d", s.HandshakeSecs, want)
	}
}

func TestParseWgBytes(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"512 KiB received", 512 * 1024},
		{"1.5 GiB sent", int64(1.5 * 1024 * 1024 * 1024)},
		{"0 B received", 0},
		{"garbage", 0},
	}
	for _, c := range cases {
		if got := parseWgBytes(c.in); got != c.want {
			t.Errorf("parseWgBytes(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
