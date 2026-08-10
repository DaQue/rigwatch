// Package pia monitors a local Private Internet Access WireGuard tunnel
// (`wg show pia`). It is deliberately fork-specific: upstream rigwatch does
// not ship VPN monitoring, so the whole feature lives in this one package and
// is wired into the web server with a single handler (see web/server.go).
//
// The only way to use it is pia.New(interval).Status() — there is no global
// state and no exported helper that can be misused.
package pia

import (
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Status is the JSON shape served at /api/pia.
type Status struct {
	Connected     bool   `json:"connected"`
	PublicKey     string `json:"public_key"`
	Endpoint      string `json:"endpoint"`
	HandshakeAgo  string `json:"handshake_ago"`
	HandshakeSecs int    `json:"handshake_secs"`
	TransferRx    int64  `json:"transfer_rx"`
	TransferTx    int64  `json:"transfer_tx"`
	TransferRxStr string `json:"transfer_rx_str"`
	TransferTxStr string `json:"transfer_tx_str"`
}

// Poller refreshes the tunnel status on a fixed interval. Safe for concurrent
// use: Status() returns a snapshot copy.
type Poller struct {
	interval time.Duration
	mu       sync.RWMutex
	last     Status
	stop     chan struct{}
	done     chan struct{}
}

// New starts a Poller that reads `wg show pia` every interval. A nil or
// non-positive interval defaults to 5s.
func New(interval time.Duration) *Poller {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	p := &Poller{
		interval: interval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go p.loop()
	return p
}

// Stop halts the background poller and waits for it to exit. Safe to call
// multiple times.
func (p *Poller) Stop() {
	select {
	case <-p.done:
		return
	default:
	}
	close(p.stop)
	<-p.done
}

func (p *Poller) loop() {
	defer close(p.done)
	p.refresh()
	t := time.NewTicker(p.interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			p.refresh()
		case <-p.stop:
			return
		}
	}
}

func (p *Poller) refresh() {
	out, err := exec.Command("wg", "show", "pia").Output()
	p.mu.Lock()
	defer p.mu.Unlock()
	if err != nil {
		// Tunnel missing or down.
		p.last = Status{Connected: false}
		return
	}
	p.last = parseWgOutput(string(out))
}

// Status returns a copy of the current tunnel state.
func (p *Poller) Status() Status {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.last
}

func parseWgOutput(out string) Status {
	s := Status{Connected: true}
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "public key:"):
			s.PublicKey = strings.TrimSpace(strings.TrimPrefix(line, "public key:"))
		case strings.HasPrefix(line, "endpoint:"):
			s.Endpoint = strings.TrimSpace(strings.TrimPrefix(line, "endpoint:"))
			if idx := strings.LastIndex(s.Endpoint, ":"); idx >= 0 {
				s.Endpoint = s.Endpoint[:idx]
			}
		case strings.HasPrefix(line, "latest handshake:"):
			raw := strings.TrimSpace(strings.TrimPrefix(line, "latest handshake:"))
			s.HandshakeAgo = raw
			raw = strings.TrimSuffix(raw, "ago")
			raw = strings.TrimSpace(raw)
			secs := 0
			parts := strings.Split(raw, ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				fields := strings.Fields(p)
				if len(fields) >= 2 {
					n, err := strconv.Atoi(fields[0])
					if err != nil {
						continue
					}
					unit := fields[1]
					if strings.HasPrefix(unit, "hour") || strings.HasPrefix(unit, "hr") {
						secs += n * 3600
					} else if strings.HasPrefix(unit, "minute") || strings.HasPrefix(unit, "min") {
						secs += n * 60
					} else if strings.HasPrefix(unit, "second") || strings.HasPrefix(unit, "sec") {
						secs += n
					}
				}
			}
			s.HandshakeSecs = secs
		case strings.HasPrefix(line, "transfer:"):
			raw := strings.TrimSpace(strings.TrimPrefix(line, "transfer:"))
			s.TransferRxStr, s.TransferTxStr, s.TransferRx, s.TransferTx = parseTransfer(raw)
		}
	}
	return s
}

func parseTransfer(raw string) (rxStr, txStr string, rxBytes, txBytes int64) {
	parts := strings.Split(raw, ",")
	if len(parts) < 2 {
		return raw, "", 0, 0
	}
	rxStr = strings.TrimSpace(parts[0])
	txStr = strings.TrimSpace(parts[1])
	rxBytes = parseWgBytes(rxStr)
	txBytes = parseWgBytes(txStr)
	return
}

func parseWgBytes(s string) int64 {
	// e.g. "841.18 MiB received" or "124.78 MiB sent"
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return 0
	}
	val, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	unit := fields[1]
	switch unit {
	case "KiB":
		return int64(val * 1024)
	case "MiB":
		return int64(val * 1024 * 1024)
	case "GiB":
		return int64(val * 1024 * 1024 * 1024)
	case "TiB":
		return int64(val * 1024 * 1024 * 1024 * 1024)
	default:
		return int64(val)
	}
}
