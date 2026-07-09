package nativeapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/log"
)

// [LeChenMusic-START:version-check]

// These are set via ldflags at build time
var (
	gitSha = "dev"
	gitTag = "dev"
)

func (api *Router) addVersionRoute(r chi.Router) {
	r.Route("/version", func(r chi.Router) {
		r.Get("/", getVersion)
		r.Get("/check", checkUpdate)
	})
}

type versionInfo struct {
	CurrentSHA   string `json:"currentSHA"`
	CurrentTag   string `json:"currentTag"`
	ServerName   string `json:"serverName"`
	ServerURL    string `json:"serverUrl"`
	GitHubAPIURL string `json:"githubApiUrl"`
}

func getVersion(w http.ResponseWriter, r *http.Request) {
	info := versionInfo{
		CurrentSHA:   gitSha,
		CurrentTag:   gitTag,
		ServerName:   "LeChenMusic",
		ServerURL:    "https://github.com/yueyoue/LeChenMusic-Server",
		GitHubAPIURL: "https://api.github.com/repos/yueyoue/LeChenMusic-Server",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": info})
}

type updateInfo struct {
	HasUpdate     bool   `json:"hasUpdate"`
	CurrentSHA    string `json:"currentSHA"`
	LatestSHA     string `json:"latestSHA"`
	LatestTag     string `json:"latestTag"`
	LatestDate    string `json:"latestDate"`
	Changelog     string `json:"changelog"`
	DownloadURL   string `json:"downloadUrl"`
	UpdateCommand string `json:"updateCommand"`
}

func checkUpdate(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{Timeout: 10 * time.Second}

	req, _ := http.NewRequest("GET", "https://api.github.com/repos/yueyoue/LeChenMusic-Server/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		log.Error(r.Context(), "Version check: GitHub API error", err)
		http.Error(w, "Failed to check updates", 500)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		checkLatestCommit(w, r)
		return
	}

	var release struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		Body        string `json:"body"`
		PublishedAt string `json:"published_at"`
		HTMLURL     string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		log.Error(r.Context(), "Version check: decode error", err)
		http.Error(w, "Failed to parse release", 500)
		return
	}

	currentSHA := gitSha
	hasUpdate := release.TagName != currentSHA && release.TagName != ""

	info := updateInfo{
		HasUpdate:     hasUpdate,
		CurrentSHA:    currentSHA,
		LatestSHA:     release.TagName,
		LatestTag:     release.TagName,
		LatestDate:    release.PublishedAt,
		Changelog:     release.Body,
		DownloadURL:   fmt.Sprintf("ghcr.io/yueyoue/lechenmusic-server:%s", release.TagName),
		UpdateCommand: fmt.Sprintf("docker pull ghcr.io/yueyoue/lechenmusic-server:%s && docker compose down && docker compose up -d", release.TagName),
	}
	if !hasUpdate {
		info.LatestSHA = currentSHA
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": info})
}

func checkLatestCommit(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/yueyoue/LeChenMusic-Server/commits?per_page=1", nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "Failed to check updates", 500)
		return
	}
	defer resp.Body.Close()

	var commits []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message   string `json:"message"`
			Committer struct {
				Date string `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&commits); err != nil || len(commits) == 0 {
		http.Error(w, "Failed to parse commits", 500)
		return
	}

	latest := commits[0]
	currentSHA := gitSha
	// Use 7-char short SHA to match Docker image tags from GitHub Actions
	// (docker/metadata-action type=sha defaults to 7 characters)
	shortSHA := latest.SHA
	if len(shortSHA) > 7 {
		shortSHA = shortSHA[:7]
	}

	hasUpdate := latest.SHA != currentSHA

	info := updateInfo{
		HasUpdate:     hasUpdate,
		CurrentSHA:    currentSHA,
		LatestSHA:     latest.SHA,
		LatestTag:     shortSHA,
		LatestDate:    latest.Commit.Committer.Date,
		Changelog:     latest.Commit.Message,
		DownloadURL:   fmt.Sprintf("ghcr.io/yueyoue/lechenmusic-server:%s", shortSHA),
		UpdateCommand: fmt.Sprintf("docker pull ghcr.io/yueyoue/lechenmusic-server:%s && docker compose down && docker compose up -d", shortSHA),
	}
	if !hasUpdate {
		info.LatestSHA = currentSHA
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": info})
}

// [LeChenMusic-END:version-check]
