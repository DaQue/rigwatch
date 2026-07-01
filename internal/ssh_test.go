package internal

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeSSHConfigFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
	return path
}

func TestParseSSHConfigParsesHostBlocks(t *testing.T) {
	dir := t.TempDir()
	configPath := writeSSHConfigFixture(t, dir, "config", `
Host web
    HostName web.example.com
    User deploy
    Port 2222
    IdentityFile id_ed25519

Host *
    User ignored
`)

	hosts, err := ParseSSHConfig(configPath)
	if err != nil {
		t.Fatalf("ParseSSHConfig returned error: %v", err)
	}

	want := []SSHHost{{
		Name:         "web",
		Hostname:     "web.example.com",
		User:         "deploy",
		Port:         "2222",
		IdentityFile: filepath.Join(os.Getenv("HOME"), ".ssh", "id_ed25519"),
	}}
	if !reflect.DeepEqual(hosts, want) {
		t.Fatalf("hosts = %#v, want %#v", hosts, want)
	}
}

func TestParseSSHConfigIncludesRelativeGlobs(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "conf.d"), 0700); err != nil {
		t.Fatalf("mkdir conf.d: %v", err)
	}
	writeSSHConfigFixture(t, filepath.Join(dir, "conf.d"), "a.conf", `
Host included
    HostName included.example.com
`)
	configPath := writeSSHConfigFixture(t, dir, "config", `
Include conf.d/*.conf
Host main
    HostName main.example.com
`)

	hosts, err := ParseSSHConfig(configPath)
	if err != nil {
		t.Fatalf("ParseSSHConfig returned error: %v", err)
	}

	gotNames := make([]string, len(hosts))
	for i, host := range hosts {
		gotNames[i] = host.Name
	}
	wantNames := []string{"included", "main"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("host names = %#v, want %#v", gotNames, wantNames)
	}
}

func TestParseSSHConfigSplitsMultipleConcreteHostPatterns(t *testing.T) {
	dir := t.TempDir()
	configPath := writeSSHConfigFixture(t, dir, "config", `
Host app db *.internal ?astion
    HostName shared.example.com
    User deploy
`)

	hosts, err := ParseSSHConfig(configPath)
	if err != nil {
		t.Fatalf("ParseSSHConfig returned error: %v", err)
	}

	gotNames := make([]string, len(hosts))
	for i, host := range hosts {
		gotNames[i] = host.Name
	}
	wantNames := []string{"app", "db"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("host names = %#v, want %#v", gotNames, wantNames)
	}
}

func TestParseSSHConfigRecursiveIncludeGuard(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	if err := os.WriteFile(configPath, []byte(`
Include config
Host loop-safe
    HostName loop.example.com
`), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	hosts, err := ParseSSHConfig(configPath)
	if err != nil {
		t.Fatalf("ParseSSHConfig returned error: %v", err)
	}
	if len(hosts) != 1 || hosts[0].Name != "loop-safe" {
		t.Fatalf("hosts = %#v, want one loop-safe host", hosts)
	}
}

func TestParseSSHConfigMissingFileReturnsEmptyHosts(t *testing.T) {
	hosts, err := ParseSSHConfig(filepath.Join(t.TempDir(), "missing-config"))
	if err != nil {
		t.Fatalf("ParseSSHConfig missing file error = %v, want nil", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("hosts = %#v, want empty", hosts)
	}
}

func TestEnsureLocalhostPrependsSelectableLocalhost(t *testing.T) {
	hosts := EnsureLocalhost([]SSHHost{{Name: "remote", Hostname: "remote.example.com", Port: "22"}})
	if len(hosts) != 2 {
		t.Fatalf("hosts len = %d, want 2: %#v", len(hosts), hosts)
	}
	if hosts[0].Name != "localhost" || !hosts[0].Local {
		t.Fatalf("first host = %#v, want synthetic local localhost", hosts[0])
	}
	if hosts[1].Name != "remote" {
		t.Fatalf("second host = %#v, want original remote", hosts[1])
	}
}

func TestEnsureLocalhostDoesNotDuplicateConfiguredLocalhost(t *testing.T) {
	configured := SSHHost{Name: "localhost", Hostname: "127.0.0.1", Port: "2222"}
	hosts := EnsureLocalhost([]SSHHost{configured})
	if !reflect.DeepEqual(hosts, []SSHHost{configured}) {
		t.Fatalf("hosts = %#v, want configured localhost unchanged", hosts)
	}
}

func TestLocalhostClientExecutesAllowedCommandWithoutSSH(t *testing.T) {
	client, err := NewSSHClient(SSHHost{Name: "localhost", Hostname: "localhost", Local: true})
	if err != nil {
		t.Fatalf("NewSSHClient(localhost) returned error: %v", err)
	}
	if client.client != nil {
		t.Fatalf("local client should not create an ssh connection")
	}

	output, err := client.ExecuteCommand("which sh")
	if err != nil {
		t.Fatalf("ExecuteCommand(which sh) returned error: %v", err)
	}
	if !strings.Contains(output, "sh") {
		t.Fatalf("output = %q, want path containing sh", output)
	}
}

func TestLocalhostCommandTimeout(t *testing.T) {
	previous := commandTimeout
	commandTimeout = 0
	t.Cleanup(func() { commandTimeout = previous })

	client, err := NewSSHClient(SSHHost{Name: "localhost", Hostname: "localhost", Local: true})
	if err != nil {
		t.Fatalf("NewSSHClient(localhost) returned error: %v", err)
	}

	_, err = client.ExecuteCommand("which sh")
	if err == nil {
		t.Fatalf("expected command timeout")
	}
	if !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("timeout error = %v", err)
	}
}
