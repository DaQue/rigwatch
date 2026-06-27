package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveLoadDeleteRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	host := SSHHost{Name: "box", Hostname: "10.0.0.5", User: "ubuntu", Port: "2222"}
	if err := SaveManagedHost(host); err != nil {
		t.Fatalf("SaveManagedHost: %v", err)
	}

	hosts, err := LoadManagedHosts()
	if err != nil {
		t.Fatalf("LoadManagedHosts: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 managed host, got %d", len(hosts))
	}
	got := hosts[0]
	if got.Name != "box" || got.Hostname != "10.0.0.5" || got.User != "ubuntu" || got.Port != "2222" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if !got.Managed {
		t.Fatalf("expected Managed=true")
	}

	// Updating the same name should replace, not duplicate.
	host.User = "root"
	if err := SaveManagedHost(host); err != nil {
		t.Fatalf("SaveManagedHost update: %v", err)
	}
	hosts, _ = LoadManagedHosts()
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host after update, got %d", len(hosts))
	}
	if hosts[0].User != "root" {
		t.Fatalf("expected updated user root, got %q", hosts[0].User)
	}

	if err := DeleteManagedHost("box"); err != nil {
		t.Fatalf("DeleteManagedHost: %v", err)
	}
	hosts, _ = LoadManagedHosts()
	if len(hosts) != 0 {
		t.Fatalf("expected 0 hosts after delete, got %d", len(hosts))
	}
}

func TestEnsureIncludeDirectiveIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	for i := 0; i < 3; i++ {
		if err := EnsureIncludeDirective(); err != nil {
			t.Fatalf("EnsureIncludeDirective: %v", err)
		}
	}

	configPath := filepath.Join(home, ".ssh", "config")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if n := strings.Count(string(data), "Include "+managedConfigName); n != 1 {
		t.Fatalf("expected exactly 1 Include directive, got %d", n)
	}

	managedPath := filepath.Join(home, ".ssh", managedConfigName)
	info, err := os.Stat(managedPath)
	if err != nil {
		t.Fatalf("stat managed file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("managed file perms = %o, want 600", info.Mode().Perm())
	}
}

func TestEnsureIncludePreservesExistingConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	original := "Host existing\n    HostName 1.2.3.4\n"
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(original), 0600); err != nil {
		t.Fatal(err)
	}

	if err := EnsureIncludeDirective(); err != nil {
		t.Fatalf("EnsureIncludeDirective: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(sshDir, "config"))
	if !strings.Contains(string(data), original) {
		t.Fatalf("original config content lost:\n%s", data)
	}
	if !strings.Contains(string(data), "Include "+managedConfigName) {
		t.Fatalf("include directive not added:\n%s", data)
	}
}

func TestLoadAllHostsDedupesManaged(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := SaveManagedHost(SSHHost{Name: "box", Hostname: "10.0.0.5", User: "ubuntu"}); err != nil {
		t.Fatalf("SaveManagedHost: %v", err)
	}

	hosts, err := LoadAllHosts()
	if err != nil {
		t.Fatalf("LoadAllHosts: %v", err)
	}

	var boxCount int
	var box SSHHost
	var haveLocalhost bool
	for _, h := range hosts {
		if h.Name == "box" {
			boxCount++
			box = h
		}
		if h.Name == "localhost" {
			haveLocalhost = true
		}
	}
	if boxCount != 1 {
		t.Fatalf("expected box exactly once (managed file is also Include'd), got %d", boxCount)
	}
	if !box.Managed {
		t.Fatalf("expected the deduped box to be the managed copy")
	}
	if !haveLocalhost {
		t.Fatalf("expected localhost to be ensured")
	}
}

func TestValidateHost(t *testing.T) {
	cases := []struct {
		name    string
		host    SSHHost
		wantErr bool
	}{
		{"ok", SSHHost{Name: "box", Port: "22"}, false},
		{"empty name", SSHHost{Name: ""}, true},
		{"space in name", SSHHost{Name: "my box"}, true},
		{"slash in name", SSHHost{Name: "a/b"}, true},
		{"bad port", SSHHost{Name: "box", Port: "abc"}, true},
		{"port out of range", SSHHost{Name: "box", Port: "70000"}, true},
		{"empty port ok", SSHHost{Name: "box", Port: ""}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateHost(tc.host)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateHost(%+v) err=%v, wantErr=%v", tc.host, err, tc.wantErr)
			}
		})
	}
}
