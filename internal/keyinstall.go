package internal

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var errKeyScanned = errors.New("host key scanned")

// EnsureLocalKeyPair returns the local public key bytes (authorized_keys format),
// generating an ed25519 keypair at ~/.ssh/id_ed25519 if one does not exist.
func EnsureLocalKeyPair() ([]byte, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return nil, fmt.Errorf("unable to create .ssh directory: %w", err)
	}

	privPath := filepath.Join(sshDir, "id_ed25519")
	pubPath := privPath + ".pub"

	if data, err := os.ReadFile(pubPath); err == nil {
		return data, nil
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("unable to generate key: %w", err)
	}

	pemBlock, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		return nil, fmt.Errorf("unable to marshal private key: %w", err)
	}
	if err := os.WriteFile(privPath, pem.EncodeToMemory(pemBlock), 0600); err != nil {
		return nil, fmt.Errorf("unable to write private key: %w", err)
	}

	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("unable to derive public key: %w", err)
	}
	pubBytes := ssh.MarshalAuthorizedKey(sshPub)
	if err := os.WriteFile(pubPath, pubBytes, 0644); err != nil {
		return nil, fmt.Errorf("unable to write public key: %w", err)
	}

	return pubBytes, nil
}

func resolveAddr(host SSHHost) string {
	hostname := host.Hostname
	if hostname == "" {
		hostname = host.Name
	}
	port := host.Port
	if port == "" {
		port = "22"
	}
	return net.JoinHostPort(hostname, port)
}

// ScanHostKey opens a connection just far enough to capture the remote host key
// and returns it alongside its SHA256 fingerprint for trust-on-first-use display.
func ScanHostKey(host SSHHost) (ssh.PublicKey, string, error) {
	addr := resolveAddr(host)

	var captured ssh.PublicKey
	cb := func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		captured = key
		return errKeyScanned
	}

	config := &ssh.ClientConfig{
		User:            "rigwatch",
		HostKeyCallback: cb,
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", addr, config)
	if client != nil {
		client.Close()
	}

	if captured != nil {
		return captured, ssh.FingerprintSHA256(captured), nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("failed to scan host key for %s: %w", addr, err)
	}
	return nil, "", fmt.Errorf("no host key received from %s", addr)
}

// AddKnownHost appends a host key to ~/.ssh/known_hosts in standard format.
func AddKnownHost(hostname, port string, key ssh.PublicKey) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return err
	}
	knownHostsPath := filepath.Join(sshDir, "known_hosts")

	if port == "" {
		port = "22"
	}
	normalized := knownhosts.Normalize(net.JoinHostPort(hostname, port))
	line := knownhosts.Line([]string{normalized}, key) + "\n"

	f, err := os.OpenFile(knownHostsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("unable to open known_hosts: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		return fmt.Errorf("unable to write known_hosts: %w", err)
	}
	return nil
}

func passwordAuthMethods(password string) []ssh.AuthMethod {
	return []ssh.AuthMethod{
		ssh.Password(password),
		ssh.KeyboardInteractive(func(name, instruction string, questions []string, echos []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i := range answers {
				answers[i] = password
			}
			return answers, nil
		}),
	}
}

// buildInstallScript returns the idempotent remote snippet that appends a public
// key (read from stdin) to ~/.ssh/authorized_keys without creating duplicates.
func buildInstallScript() string {
	return `sh -c 'umask 077; mkdir -p ~/.ssh && touch ~/.ssh/authorized_keys && k=$(cat) && { grep -qxF "$k" ~/.ssh/authorized_keys || printf "%s\n" "$k" >> ~/.ssh/authorized_keys; }'`
}

// InstallPublicKey connects with a password and appends pubKey to the remote's
// authorized_keys. The host key must already be trusted (see ScanHostKey /
// AddKnownHost). This is a dedicated path independent of the monitoring allowlist.
func InstallPublicKey(host SSHHost, password string, pubKey []byte) error {
	hostKeyCallback, err := getHostKeyCallback()
	if err != nil {
		return fmt.Errorf("failed to setup host key verification: %w", err)
	}

	user := host.User
	if user == "" {
		user = getValidatedUsername()
	}

	config := &ssh.ClientConfig{
		User:            user,
		Auth:            passwordAuthMethods(password),
		HostKeyCallback: hostKeyCallback,
		Timeout:         15 * time.Second,
	}

	addr := resolveAddr(host)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", addr, err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	session.Stdin = bytes.NewReader(pubKey)
	if output, err := session.CombinedOutput(buildInstallScript()); err != nil {
		return fmt.Errorf("failed to install key: %w: %s", err, string(output))
	}
	return nil
}
