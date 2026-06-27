package internal

import (
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestEnsureLocalKeyPairGenerates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	pub, err := EnsureLocalKeyPair()
	if err != nil {
		t.Fatalf("EnsureLocalKeyPair: %v", err)
	}
	if !strings.HasPrefix(string(pub), "ssh-ed25519 ") {
		t.Fatalf("unexpected public key format: %q", string(pub))
	}

	privPath := filepath.Join(home, ".ssh", "id_ed25519")
	info, err := os.Stat(privPath)
	if err != nil {
		t.Fatalf("private key not written: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("private key perms = %o, want 600", info.Mode().Perm())
	}

	// Second call must reuse the existing key, not regenerate.
	pub2, err := EnsureLocalKeyPair()
	if err != nil {
		t.Fatalf("EnsureLocalKeyPair second call: %v", err)
	}
	if string(pub) != string(pub2) {
		t.Fatalf("key changed across calls; expected reuse")
	}
}

func TestBuildInstallScriptIdempotent(t *testing.T) {
	script := buildInstallScript()
	for _, want := range []string{"mkdir -p ~/.ssh", "authorized_keys", "grep -qxF", "umask 077"} {
		if !strings.Contains(script, want) {
			t.Fatalf("install script missing %q:\n%s", want, script)
		}
	}
}

func TestPasswordAuthMethods(t *testing.T) {
	methods := passwordAuthMethods("hunter2")
	if len(methods) != 2 {
		t.Fatalf("expected password + keyboard-interactive, got %d methods", len(methods))
	}
}

func TestAddKnownHost(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}

	if err := AddKnownHost("example.com", "22", sshPub); err != nil {
		t.Fatalf("AddKnownHost: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".ssh", "known_hosts"))
	if err != nil {
		t.Fatalf("read known_hosts: %v", err)
	}
	line := string(data)
	if !strings.Contains(line, "example.com") {
		t.Fatalf("known_hosts missing hostname: %q", line)
	}
	if !strings.Contains(line, "ssh-ed25519") {
		t.Fatalf("known_hosts missing key type: %q", line)
	}
}
