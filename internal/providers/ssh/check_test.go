package ssh

import (
	"context"
	"net"
	"testing"

	"github.com/jmreicha/cfgctl/internal/core"
)

func TestSSHCheck_Disabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	p := &Provider{config: cfg}

	results, err := p.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no results for disabled provider, got %d", len(results))
	}
}

func TestSSHCheck_NoHosts(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Hosts = nil
	p := &Provider{config: cfg}

	results, err := p.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected no results for empty hosts, got %d", len(results))
	}
}

func TestSSHCheck_ReachableHost(t *testing.T) {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer func() { _ = ln.Close() }()

	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("expected *net.TCPAddr")
	}
	cfg := DefaultConfig()
	cfg.Hosts = []HostConfig{{
		Host:     "test-host",
		Hostname: "127.0.0.1",
		Port:     tcpAddr.Port,
	}}
	p := &Provider{config: cfg}

	results, err := p.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != core.CheckStatusOK {
		t.Errorf("expected OK, got %v (note: %s)", results[0].Status, results[0].Note)
	}
	if results[0].Target != "test-host" {
		t.Errorf("expected target %q, got %q", "test-host", results[0].Target)
	}
	if results[0].Latency == 0 {
		t.Error("expected non-zero latency for successful dial")
	}
}

func TestSSHCheck_UnreachableHost(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Hosts = []HostConfig{{
		Host:     "bad-host",
		Hostname: "127.0.0.1",
		Port:     1, // nothing listening
	}}
	p := &Provider{config: cfg}

	results, err := p.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != core.CheckStatusFail {
		t.Errorf("expected Fail, got %v", results[0].Status)
	}
	if results[0].Note == "" {
		t.Error("expected non-empty note for failed dial")
	}
}

func TestSSHCheck_DefaultPort(t *testing.T) {
	// Port 0 in config should fall back to default SSH port (22).
	// We can't connect on port 22 in tests, but we verify the attempt
	// uses the right target name and returns a Fail (not a panic).
	cfg := DefaultConfig()
	cfg.Hosts = []HostConfig{{
		Host:     "my-host",
		Hostname: "127.0.0.1",
		Port:     0, // should use defaultSSHPort
	}}
	p := &Provider{config: cfg}

	results, err := p.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Target != "my-host" {
		t.Errorf("expected target %q, got %q", "my-host", results[0].Target)
	}
}

func TestSSHCheck_HostnameUsedOverHost(t *testing.T) {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer func() { _ = ln.Close() }()

	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("expected *net.TCPAddr")
	}
	cfg := DefaultConfig()
	cfg.Hosts = []HostConfig{{
		Host:     "alias",
		Hostname: "127.0.0.1", // real address — if Hostname weren't used, "alias" would fail to resolve
		Port:     tcpAddr.Port,
	}}
	p := &Provider{config: cfg}

	results, err := p.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if results[0].Status != core.CheckStatusOK {
		t.Errorf("expected OK when Hostname is used for dial, got %v", results[0].Status)
	}
}

func TestSSHCheck_MultipleHosts(t *testing.T) {
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer func() { _ = ln.Close() }()

	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("expected *net.TCPAddr")
	}
	cfg := DefaultConfig()
	cfg.Hosts = []HostConfig{
		{Host: "good", Hostname: "127.0.0.1", Port: tcpAddr.Port},
		{Host: "bad", Hostname: "127.0.0.1", Port: 1},
	}
	p := &Provider{config: cfg}

	results, err := p.Check(context.Background())
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	byTarget := map[string]core.CheckStatus{}
	for _, r := range results {
		byTarget[r.Target] = r.Status
	}
	if byTarget["good"] != core.CheckStatusOK {
		t.Errorf("expected good host to be OK, got %v", byTarget["good"])
	}
	if byTarget["bad"] != core.CheckStatusFail {
		t.Errorf("expected bad host to be Fail, got %v", byTarget["bad"])
	}
}
