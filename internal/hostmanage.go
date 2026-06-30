package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const managedConfigName = "rigwatch_config"

const managedConfigHeader = `# Managed by rigwatch — do not edit by hand.
# Hosts here are added, edited, and removed from the rigwatch connection manager.
`

// ManagedConfigPath returns the path to the rigwatch-managed SSH config file.
func ManagedConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh", managedConfigName), nil
}

func userSSHDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh"), nil
}

// EnsureIncludeDirective makes sure ~/.ssh/config pulls in the rigwatch-managed
// file via an Include directive, creating ~/.ssh and the managed file if needed.
// It is idempotent.
func EnsureIncludeDirective() error {
	sshDir, err := userSSHDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("unable to create .ssh directory: %w", err)
	}

	managedPath := filepath.Join(sshDir, managedConfigName)
	if _, err := os.Stat(managedPath); os.IsNotExist(err) {
		if err := os.WriteFile(managedPath, []byte(managedConfigHeader), 0600); err != nil {
			return fmt.Errorf("unable to create managed config: %w", err)
		}
	}

	configPath := filepath.Join(sshDir, "config")
	existing, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("unable to read ssh config: %w", err)
	}

	if includeDirectivePresent(string(existing)) {
		return nil
	}

	includeLine := "Include " + managedConfigName + "\n"
	var updated string
	if len(existing) == 0 {
		updated = includeLine
	} else {
		// OpenSSH applies the first value obtained for each keyword, so the
		// Include belongs at the top to take effect alongside hand-written hosts.
		updated = includeLine + string(existing)
	}

	return writeFileAtomic(configPath, []byte(updated), 0600)
}

func includeDirectivePresent(config string) bool {
	for _, raw := range strings.Split(config, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.ToLower(fields[0]) != "include" {
			continue
		}
		for _, arg := range fields[1:] {
			if arg == managedConfigName || filepath.Base(arg) == managedConfigName {
				return true
			}
		}
	}
	return false
}

// LoadManagedHosts parses the rigwatch-managed config file, returning the hosts
// it defines with Managed set to true. A missing file yields no hosts.
func LoadManagedHosts() ([]SSHHost, error) {
	managedPath, err := ManagedConfigPath()
	if err != nil {
		return nil, err
	}
	hosts, err := parseSSHConfigRecursive(managedPath, make(map[string]bool))
	if err != nil {
		return nil, err
	}
	for i := range hosts {
		hosts[i].Managed = true
	}
	return hosts, nil
}

// LoadAllHosts returns the full host list shown to the user: rigwatch-managed
// hosts first (so they win and carry Managed=true), then any remaining hosts
// from ~/.ssh/config, with localhost ensured. Because the managed file is
// Include'd into ~/.ssh/config, managed hosts also appear in the parsed config;
// putting the managed copies first de-dupes them with the correct flag.
func LoadAllHosts() ([]SSHHost, error) {
	managed, err := LoadManagedHosts()
	if err != nil {
		return nil, err
	}
	configHosts, err := ParseSSHConfig("")
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	all := make([]SSHHost, 0, len(managed)+len(configHosts))
	for _, h := range managed {
		if !seen[h.Name] {
			all = append(all, h)
			seen[h.Name] = true
		}
	}
	for _, h := range configHosts {
		if !seen[h.Name] {
			all = append(all, h)
			seen[h.Name] = true
		}
	}

	return EnsureLocalhost(all), nil
}

// SaveManagedHost adds the host to the managed config, or replaces an existing
// managed host with the same name.
func SaveManagedHost(host SSHHost) error {
	if err := ValidateHost(host); err != nil {
		return err
	}
	if err := EnsureIncludeDirective(); err != nil {
		return err
	}

	hosts, err := LoadManagedHosts()
	if err != nil {
		return err
	}

	host.Managed = true
	replaced := false
	for i := range hosts {
		if hosts[i].Name == host.Name {
			hosts[i] = host
			replaced = true
			break
		}
	}
	if !replaced {
		hosts = append(hosts, host)
	}

	return writeManagedHosts(hosts)
}

// DeleteManagedHost removes a host from the managed config by name. Removing a
// name that is not managed is a no-op.
func DeleteManagedHost(name string) error {
	hosts, err := LoadManagedHosts()
	if err != nil {
		return err
	}

	filtered := hosts[:0]
	for _, h := range hosts {
		if h.Name != name {
			filtered = append(filtered, h)
		}
	}

	return writeManagedHosts(filtered)
}

func writeManagedHosts(hosts []SSHHost) error {
	managedPath, err := ManagedConfigPath()
	if err != nil {
		return err
	}

	var b strings.Builder
	b.WriteString(managedConfigHeader)
	for _, h := range hosts {
		b.WriteString("\n")
		b.WriteString(serializeHost(h))
	}

	return writeFileAtomic(managedPath, []byte(b.String()), 0600)
}

func serializeHost(h SSHHost) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Host %s\n", h.Name)
	if h.Hostname != "" {
		fmt.Fprintf(&b, "    HostName %s\n", h.Hostname)
	}
	if h.User != "" {
		fmt.Fprintf(&b, "    User %s\n", h.User)
	}
	if h.Port != "" {
		fmt.Fprintf(&b, "    Port %s\n", h.Port)
	}
	if h.IdentityFile != "" {
		fmt.Fprintf(&b, "    IdentityFile %s\n", h.IdentityFile)
	}
	if h.PasswordAuth {
		fmt.Fprintf(&b, "    PreferredAuthentications password\n")
	}
	return b.String()
}

// ValidateHost checks user-supplied host fields before they are persisted.
func ValidateHost(h SSHHost) error {
	name := strings.TrimSpace(h.Name)
	if name == "" {
		return fmt.Errorf("host name is required")
	}
	if strings.ContainsAny(name, " \t/\\") {
		return fmt.Errorf("host name must not contain whitespace or slashes")
	}
	if h.Port != "" {
		port, err := strconv.Atoi(h.Port)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("port must be a number between 1 and 65535")
		}
	}
	return nil
}

// writeFileAtomic writes data to path via a temp file in the same directory
// followed by a rename, so readers never see a partially written file.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".rigwatch-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
