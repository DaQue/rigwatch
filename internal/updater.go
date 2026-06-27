package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	githubAPIURL       = "https://api.github.com/repos/allisonhere/rigwatch/releases/latest"
	updateCheckTimeout = 3 * time.Second
)

type UpdateInfo struct {
	Available      bool
	LatestVersion  string
	CurrentVersion string
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

func CheckForUpdates() UpdateInfo {
	currentVersion := Version
	if currentVersion == "dev" {
		return UpdateInfo{Available: false, CurrentVersion: currentVersion}
	}

	latestVersion, err := fetchLatestVersion()
	if err != nil {
		return UpdateInfo{Available: false, CurrentVersion: currentVersion}
	}

	needsUpdate := compareVersions(currentVersion, latestVersion)

	return UpdateInfo{
		Available:      needsUpdate,
		LatestVersion:  latestVersion,
		CurrentVersion: currentVersion,
	}
}

func fetchLatestVersion() (string, error) {
	client := &http.Client{
		Timeout: updateCheckTimeout,
	}

	resp, err := client.Get(githubAPIURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github api returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	return release.TagName, nil
}

func compareVersions(current, latest string) bool {
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")

	currentBase := strings.Split(strings.Split(current, "-")[0], "+")[0]
	latestBase := strings.Split(strings.Split(latest, "-")[0], "+")[0]

	currentParts := strings.Split(currentBase, ".")
	latestParts := strings.Split(latestBase, ".")

	for len(currentParts) < 3 {
		currentParts = append(currentParts, "0")
	}
	for len(latestParts) < 3 {
		latestParts = append(latestParts, "0")
	}

	for i := 0; i < 3; i++ {
		var currentNum, latestNum int
		fmt.Sscanf(currentParts[i], "%d", &currentNum)
		fmt.Sscanf(latestParts[i], "%d", &latestNum)

		if latestNum > currentNum {
			return true
		} else if latestNum < currentNum {
			return false
		}
	}

	return false
}
