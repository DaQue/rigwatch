package internal

import (
	"strings"
	"testing"
)

func TestSerializeHostPasswordAuth(t *testing.T) {
	out := serializeHost(SSHHost{Name: "box", Hostname: "10.0.0.5", User: "ubuntu", PasswordAuth: true})
	if !strings.Contains(out, "PreferredAuthentications password") {
		t.Fatalf("expected PreferredAuthentications directive, got:\n%s", out)
	}

	// A key-auth host must not emit the directive.
	out = serializeHost(SSHHost{Name: "box", Hostname: "10.0.0.5", User: "ubuntu"})
	if strings.Contains(out, "PreferredAuthentications") {
		t.Fatalf("key-auth host should not emit PreferredAuthentications, got:\n%s", out)
	}
}

func TestPasswordAuthRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	host := SSHHost{Name: "pwbox", Hostname: "10.0.0.9", User: "ubuntu", Port: "22", PasswordAuth: true}
	if err := SaveManagedHost(host); err != nil {
		t.Fatalf("SaveManagedHost: %v", err)
	}

	hosts, err := LoadManagedHosts()
	if err != nil {
		t.Fatalf("LoadManagedHosts: %v", err)
	}
	if len(hosts) != 1 || !hosts[0].PasswordAuth {
		t.Fatalf("PasswordAuth did not round-trip: %+v", hosts)
	}
}

func TestPasswordAuthReturnsTwoMethods(t *testing.T) {
	methods := passwordAuth("hunter2")
	if len(methods) != 2 {
		t.Fatalf("passwordAuth returned %d methods, want 2 (password + keyboard-interactive)", len(methods))
	}
}
