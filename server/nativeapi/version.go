package nativeapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/consts"
	"github.com/navidrome/navidrome/log"
)

// [LeChenMusic-START:version-check]

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
	sha := consts.GitSHA
	if sha == "" || sha == "dev" || sha == "unknown" {
		sha = "dev"
	}
	tag := consts.GitTag
	if tag == "" || tag == "dev" || tag == "unknown" {
		tag = "dev"
	}
	info := versionInfo{
		CurrentSHA:   sha,
		CurrentTag:   tag,
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

// Generate a clean update command that works with any docker-compose setup
func buildUpdateCommand(imageTag string) string {
	image := "ghcr.io/yueyoue/lechenmusic-server:" + imageTag
	return fmt.Sprintf("docker pull %s && docker compose down && docker compose up -d", image)
}

func checkUpdate(w http.ResponseWriter, r *http.Request) {
	sha := consts.GitSHA
	if sha == "" || sha == "unknown" {
		sha = "dev"
	}
	tag := consts.GitTag
	if tag == "" || tag == "dev" || tag == "unknown" {
		tag = "dev"
	}

	client := &http.Client{Timeout: 10 * time.Second}

	// Try latest release first
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/yueyoue/LeChenMusic-Server/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		log.Error(r.Context(), "Version check: GitHub API error", err)
		http.Error(w, "Failed to check updates", 500)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		var release struct {
			TagName     string `json:"tag_name"`
			Name        string `json:"name"`
			Body        string `json:"body"`
			PublishedAt string `json:"published_at"`
			HTMLURL     string `json:"html_url"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&release); err == nil {
			latestShortSHA := release.TagName
			if len(latestShortSHA) > 7 {
				latestShortSHA = latestShortSHA[:7]
			}
			currentShortSHA := sha
			if len(currentShortSHA) > 7 {
				currentShortSHA = currentShortSHA[:7]
			}

			hasUpdate := !strings.EqualFold(currentShortSHA, latestShortSHA) && sha != "dev"

			info := updateInfo{
				HasUpdate:     hasUpdate,
				CurrentSHA:    sha,
				LatestSHA:     release.TagName,
				LatestTag:     release.TagName,
				LatestDate:    release.PublishedAt,
				Changelog:     release.Body,
				DownloadURL:   "ghcr.io/yueyoue/lechenmusic-server:latest",
				UpdateCommand: buildUpdateCommand("latest"),
			}
			if !hasUpdate {
				info.LatestSHA = sha
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"data": info})
			return
		}
	}

	// Fallback: check latest commit
	checkLatestCommit(w, r, sha)
}

func checkLatestCommit(w http.ResponseWriter, r *http.Request, currentSHA string) {
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
	shortSHA := latest.SHA
	if len(shortSHA) > 7 {
		shortSHA = shortSHA[:7]
	}
	currentShortSHA := currentSHA
	if len(currentShortSHA) > 7 {
		currentShortSHA = currentShortSHA[:7]
	}

	hasUpdate := !strings.EqualFold(currentShortSHA, shortSHA) && currentSHA != "dev"

	info := updateInfo{
		HasUpdate:     hasUpdate,
		CurrentSHA:    currentSHA,
		LatestSHA:     latest.SHA,
		LatestTag:     shortSHA,
		LatestDate:    latest.Commit.Committer.Date,
		Changelog:     latest.Commit.Message,
		DownloadURL:   "ghcr.io/yueyoue/lechenmusic-server:latest",
		UpdateCommand: buildUpdateCommand("latest"),
	}
	if !hasUpdate {
		info.LatestSHA = currentSHA
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": info})
}

// [LeChenMusic-END:version-check]
