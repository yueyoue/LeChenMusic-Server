package nativeapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
		r.Post("/update", oneClickUpdate)
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

func checkUpdate(w http.ResponseWriter, r *http.Request) {
	sha := consts.GitSHA
	if sha == "" || sha == "unknown" {
		sha = "dev"
	}
	tag := consts.GitTag
	if tag == "" || tag == "unknown" {
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
			// Compare: check if current SHA matches latest release tag
			latestShortSHA := release.TagName
			if len(latestShortSHA) > 8 {
				latestShortSHA = latestShortSHA[:8]
			}
			currentShortSHA := sha
			if len(currentShortSHA) > 8 {
				currentShortSHA = currentShortSHA[:8]
			}

			// hasUpdate if current SHA doesn't match latest release tag
			hasUpdate := !strings.EqualFold(currentShortSHA, latestShortSHA) && sha != "dev"

			info := updateInfo{
				HasUpdate:     hasUpdate,
				CurrentSHA:    sha,
				LatestSHA:     release.TagName,
				LatestTag:     release.TagName,
				LatestDate:    release.PublishedAt,
				Changelog:     release.Body,
				DownloadURL:   "ghcr.io/yueyoue/lechenmusic-server:latest",
				UpdateCommand: "docker pull ghcr.io/yueyoue/lechenmusic-server:latest && docker compose down && docker compose up -d",
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
	if len(shortSHA) > 8 {
		shortSHA = shortSHA[:8]
	}
	currentShortSHA := currentSHA
	if len(currentShortSHA) > 8 {
		currentShortSHA = currentShortSHA[:8]
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
		UpdateCommand: "docker pull ghcr.io/yueyoue/lechenmusic-server:latest && docker compose down && docker compose up -d",
	}
	if !hasUpdate {
		info.LatestSHA = currentSHA
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"data": info})
}

func oneClickUpdate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		json.NewEncoder(w).Encode(map[string]any{"error": "streaming not supported"})
		return
	}

	sendSSE := func(event, data string) {
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		flusher.Flush()
	}

	// Check if running inside Docker by looking for /.dockerenv or checking cgroup
	isDocker := false
	if _, err := os.Stat("/.dockerenv"); err == nil {
		isDocker = true
	} else if cgroup, err := os.ReadFile("/proc/1/cgroup"); err == nil && strings.Contains(string(cgroup), "docker") {
		isDocker = true
	} else if cgroup, err := os.ReadFile("/proc/1/cgroup"); err == nil && strings.Contains(string(cgroup), "kubepods") {
		isDocker = true
	}

	if isDocker {
		// Running inside Docker container - can't run docker commands
		// Provide the restart command for the host
		sendSSE("log", "🔍 检测到运行在 Docker 容器中")
		sendSSE("log", "")
		sendSSE("log", "⚠️ 容器内无法执行 docker 命令，请在宿主机上执行以下命令完成更新：")
		sendSSE("log", "")
		sendSSE("restart_cmd", "docker pull ghcr.io/yueyoue/lechenmusic-server:latest && docker stop lechen-music && docker rm lechen-music && docker run -d --name lechen-music --restart unless-stopped -p 3334:3334 -e TZ=Asia/Shanghai -e ND_PORT=3334 -v /vol2/1000/Docker/lechen-music/data:/data -v /vol2/1000/音乐/抖音流行歌曲1:/music:ro -v /vol2/1000/有声读物/有声读物:/audiobooks:ro ghcr.io/yueyoue/lechenmusic-server:latest")
		sendSSE("done", "")
		return
	}

	// Not in Docker - try direct binary update approach
	sendSSE("log", "🔄 正在检查最新版本...")

	// Get latest release info
	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/yueyoue/LeChenMusic-Server/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		sendSSE("error", "❌ 获取版本信息失败: "+err.Error())
		sendSSE("done", "")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		sendSSE("error", fmt.Sprintf("❌ GitHub API 返回状态码: %d", resp.StatusCode))
		sendSSE("done", "")
		return
	}

	var release struct {
		TagName string `json:"tag_name"`
		Name    string `json:"name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		sendSSE("error", "❌ 解析版本信息失败: "+err.Error())
		sendSSE("done", "")
		return
	}

	sendSSE("log", fmt.Sprintf("📦 最新版本: %s (%s)", release.Name, release.TagName))
	sendSSE("log", "")
	sendSSE("log", "⚠️ 请在服务器上执行以下命令完成更新：")
	sendSSE("log", "")
	sendSSE("restart_cmd", "docker pull ghcr.io/yueyoue/lechenmusic-server:latest && docker compose down && docker compose up -d")
	sendSSE("done", "")
}



// [LeChenMusic-END:version-check]
