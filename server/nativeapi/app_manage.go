package nativeapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/navidrome/navidrome/log"
)

// [LeChenMusic-START:app-management]

func (api *Router) addAppManageRoute(r chi.Router) {
	h := &appManageHandler{ds: api.ds}
	r.Route("/app", func(r chi.Router) {
		r.Get("/config", h.getConfig)
		r.Put("/config", h.updateConfig)
		r.Post("/apk", h.uploadApk)
		r.Get("/apk/download", h.downloadApk)
		r.Get("/apk/info", h.getApkInfo)
		r.Post("/splash", h.uploadSplash)
		r.Post("/slide", h.addSlide)                        // Legacy: adds to music slides
		r.Post("/slide/music", h.addMusicSlide)              // Music homepage slide
		r.Post("/slide/audiobook", h.addAudiobookSlide)       // Audiobook homepage slide
		r.Delete("/slide/{id}", h.deleteSlide)                // Legacy: delete from all
		r.Delete("/slide/music/{id}", h.deleteMusicSlide)     // Delete music slide
		r.Delete("/slide/audiobook/{id}", h.deleteAudiobookSlide) // Delete audiobook slide
	})
}

type appManageHandler struct {
	ds interface{}
}

// AppConfig stores app-related configuration
type AppConfig struct {
	VersionName    string        `json:"versionName"`
	VersionCode    int           `json:"versionCode"`
	ApkFileName    string        `json:"apkFileName"`
	ApkFileSize    int64         `json:"apkFileSize"`
	ApkUploadTime  string        `json:"apkUploadTime"`
	UpdateLog      string        `json:"updateLog"`
	ForceUpdate    bool          `json:"forceUpdate"`
	SplashImageURL string        `json:"splashImageUrl"`
	SplashDuration int           `json:"splashDuration"` // seconds
	Slides         []SlideConfig `json:"slides"`         // Deprecated: use MusicSlides/AudiobookSlides
	MusicSlides    []SlideConfig `json:"musicSlides"`     // Music homepage carousel slides
	AudiobookSlides []SlideConfig `json:"audiobookSlides"` // Audiobook homepage carousel slides
	ServerURL      string        `json:"serverUrl"`       // Default server URL for app
}

type SlideConfig struct {
	ID       string `json:"id"`
	ImageURL string `json:"imageUrl"`
	Title    string `json:"title"`
	Link     string `json:"link"` // optional deep link
	Sort     int    `json:"sort"`
}

// getConfig returns the current app configuration
func (h *appManageHandler) getConfig(w http.ResponseWriter, r *http.Request) {
	config := loadAppConfig()
	writeJSON(w, map[string]any{"data": config})
}

// updateConfig updates the app configuration (admin only)
func (h *appManageHandler) updateConfig(w http.ResponseWriter, r *http.Request) {
	var config AppConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	saveAppConfig(&config)
	writeJSON(w, map[string]any{"data": config, "status": "ok"})
}

// uploadApk handles APK file upload
func (h *appManageHandler) uploadApk(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(200 << 20) // 200MB max
	file, _, err := r.FormFile("apk")
	if err != nil {
		http.Error(w, "No file uploaded", 400)
		return
	}
	defer file.Close()

	// Get version info from form
	versionName := r.FormValue("versionName")
	versionCode := r.FormValue("versionCode")
	updateLog := r.FormValue("updateLog")
	forceUpdate := r.FormValue("forceUpdate") == "true"

	if versionName == "" {
		versionName = "1.0.0"
	}

	// Save APK file
	uploadDir := getAppUploadDir()
	os.MkdirAll(uploadDir, 0755)

	fileName := fmt.Sprintf("lechenmusic-v%s.apk", versionName)
	filePath := filepath.Join(uploadDir, fileName)

	dst, err := os.Create(filePath)
	if err != nil {
		http.Error(w, "Failed to save file", 500)
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		http.Error(w, "Failed to write file", 500)
		return
	}

	// Update config
	config := loadAppConfig()
	config.VersionName = versionName
	if versionCode != "" {
		fmt.Sscanf(versionCode, "%d", &config.VersionCode)
	}
	config.ApkFileName = fileName
	config.ApkFileSize = written
	config.ApkUploadTime = time.Now().Format("2006-01-02 15:04:05")
	config.UpdateLog = updateLog
	config.ForceUpdate = forceUpdate
	saveAppConfig(&config)

	log.Info(r.Context(), "APK uploaded", "file", fileName, "size", written)

	writeJSON(w, map[string]any{
		"status":      "ok",
		"fileName":    fileName,
		"fileSize":    written,
		"versionName": versionName,
	})
}

// downloadApk serves the APK file
func (h *appManageHandler) downloadApk(w http.ResponseWriter, r *http.Request) {
	config := loadAppConfig()
	if config.ApkFileName == "" {
		http.Error(w, "No APK available", 404)
		return
	}

	filePath := filepath.Join(getAppUploadDir(), config.ApkFileName)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "APK file not found", 404)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", config.ApkFileName))
	http.ServeFile(w, r, filePath)
}

// getApkInfo returns current APK info
func (h *appManageHandler) getApkInfo(w http.ResponseWriter, r *http.Request) {
	config := loadAppConfig()
	writeJSON(w, map[string]any{
		"data": map[string]any{
			"versionName": config.VersionName,
			"versionCode": config.VersionCode,
			"fileSize":    config.ApkFileSize,
			"uploadTime":  config.ApkUploadTime,
			"updateLog":   config.UpdateLog,
			"forceUpdate": config.ForceUpdate,
			"downloadUrl": "/api/app/apk/download",
		},
	})
}

// uploadSplash handles splash screen image upload
func (h *appManageHandler) uploadSplash(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20) // 10MB max
	file, header, err := r.FormFile("image")
	if err != nil {
		http.Error(w, "No file uploaded", 400)
		return
	}
	defer file.Close()

	uploadDir := getAppUploadDir()
	os.MkdirAll(uploadDir, 0755)

	// Determine extension
	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".jpg"
	}
	fileName := "splash" + ext
	filePath := filepath.Join(uploadDir, fileName)

	dst, err := os.Create(filePath)
	if err != nil {
		http.Error(w, "Failed to save file", 500)
		return
	}
	defer dst.Close()
	io.Copy(dst, file)

	// Update config
	config := loadAppConfig()
	config.SplashImageURL = "/api/app/splash/" + fileName
	if dur := r.FormValue("duration"); dur != "" {
		fmt.Sscanf(dur, "%d", &config.SplashDuration)
	}
	if config.SplashDuration == 0 {
		config.SplashDuration = 3
	}
	saveAppConfig(&config)

	writeJSON(w, map[string]any{"status": "ok", "url": config.SplashImageURL})
}

// addSlide adds a new carousel slide
func (h *appManageHandler) addSlide(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20)

	var slide SlideConfig
	imageFile, _, imageErr := r.FormFile("image")

	slide.Title = r.FormValue("title")
	slide.Link = r.FormValue("link")
	fmt.Sscanf(r.FormValue("sort"), "%d", &slide.Sort)

	config := loadAppConfig()
	slide.ID = fmt.Sprintf("slide_%d", time.Now().UnixNano())

	if imageErr == nil {
		defer imageFile.Close()
		uploadDir := getAppUploadDir()
		os.MkdirAll(uploadDir, 0755)
		fileName := slide.ID + ".jpg"
		filePath := filepath.Join(uploadDir, fileName)
		dst, _ := os.Create(filePath)
		if dst != nil {
			io.Copy(dst, imageFile)
			dst.Close()
		}
		slide.ImageURL = "/api/app/slide/" + fileName
	} else {
		// Try image URL from form
		slide.ImageURL = r.FormValue("imageUrl")
	}

	config.Slides = append(config.Slides, slide)
	saveAppConfig(&config)

	writeJSON(w, map[string]any{"status": "ok", "data": slide})
}

// deleteSlide removes a carousel slide (legacy: searches all slide groups)
func (h *appManageHandler) deleteSlide(w http.ResponseWriter, r *http.Request) {
	slideID := chi.URLParam(r, "id")
	config := loadAppConfig()

	// Search in all groups for backward compatibility
	config.MusicSlides = removeSlideFromList(config.MusicSlides, slideID)
	config.AudiobookSlides = removeSlideFromList(config.AudiobookSlides, slideID)
	config.Slides = removeSlideFromList(config.Slides, slideID)
	saveAppConfig(&config)

	writeJSON(w, map[string]any{"status": "ok"})
}

// addMusicSlide adds a slide to the music homepage carousel
func (h *appManageHandler) addMusicSlide(w http.ResponseWriter, r *http.Request) {
	slide := h.parseSlideFromRequest(r)
	config := loadAppConfig()
	config.MusicSlides = append(config.MusicSlides, slide)
	saveAppConfig(&config)
	writeJSON(w, map[string]any{"status": "ok", "data": slide})
}

// addAudiobookSlide adds a slide to the audiobook homepage carousel
func (h *appManageHandler) addAudiobookSlide(w http.ResponseWriter, r *http.Request) {
	slide := h.parseSlideFromRequest(r)
	config := loadAppConfig()
	config.AudiobookSlides = append(config.AudiobookSlides, slide)
	saveAppConfig(&config)
	writeJSON(w, map[string]any{"status": "ok", "data": slide})
}

// deleteMusicSlide removes a slide from the music homepage carousel
func (h *appManageHandler) deleteMusicSlide(w http.ResponseWriter, r *http.Request) {
	slideID := chi.URLParam(r, "id")
	config := loadAppConfig()
	config.MusicSlides = removeSlideFromList(config.MusicSlides, slideID)
	saveAppConfig(&config)
	writeJSON(w, map[string]any{"status": "ok"})
}

// deleteAudiobookSlide removes a slide from the audiobook homepage carousel
func (h *appManageHandler) deleteAudiobookSlide(w http.ResponseWriter, r *http.Request) {
	slideID := chi.URLParam(r, "id")
	config := loadAppConfig()
	config.AudiobookSlides = removeSlideFromList(config.AudiobookSlides, slideID)
	saveAppConfig(&config)
	writeJSON(w, map[string]any{"status": "ok"})
}

// parseSlideFromRequest parses a slide upload request (shared by music/audiobook)
func (h *appManageHandler) parseSlideFromRequest(r *http.Request) SlideConfig {
	r.ParseMultipartForm(10 << 20)

	var slide SlideConfig
	imageFile, _, imageErr := r.FormFile("image")

	slide.Title = r.FormValue("title")
	slide.Link = r.FormValue("link")
	fmt.Sscanf(r.FormValue("sort"), "%d", &slide.Sort)

	slide.ID = fmt.Sprintf("slide_%d", time.Now().UnixNano())

	if imageErr == nil {
		defer imageFile.Close()
		uploadDir := getAppUploadDir()
		os.MkdirAll(uploadDir, 0755)
		fileName := slide.ID + ".jpg"
		filePath := filepath.Join(uploadDir, fileName)
		dst, _ := os.Create(filePath)
		if dst != nil {
			io.Copy(dst, imageFile)
			dst.Close()
		}
		slide.ImageURL = "/api/app/slide/" + fileName
	} else {
		slide.ImageURL = r.FormValue("imageUrl")
	}

	return slide
}

// removeSlideFromList removes a slide by ID from a list
func removeSlideFromList(slides []SlideConfig, slideID string) []SlideConfig {
	var result []SlideConfig
	for _, s := range slides {
		if s.ID != slideID {
			result = append(result, s)
		}
	}
	return result
}

// --- Config persistence ---

func getAppUploadDir() string {
	// Use a data directory for uploaded files
	return filepath.Join("data", "app-uploads")
}

func getAppConfigPath() string {
	return filepath.Join("data", "app-config.json")
}

func loadAppConfig() AppConfig {
	config := AppConfig{
		VersionName:     "1.0.0",
		SplashDuration:  3,
		Slides:          []SlideConfig{},
		MusicSlides:     []SlideConfig{},
		AudiobookSlides: []SlideConfig{},
		ServerURL:       "http://j.tthsdd.top:3334",
	}

	data, err := os.ReadFile(getAppConfigPath())
	if err != nil {
		return config
	}
	json.Unmarshal(data, &config)

	// Backward compatibility: migrate legacy Slides to MusicSlides if MusicSlides is empty
	if len(config.MusicSlides) == 0 && len(config.Slides) > 0 {
		config.MusicSlides = config.Slides
	}
	// Ensure slices are never nil (for JSON serialization)
	if config.Slides == nil {
		config.Slides = []SlideConfig{}
	}
	if config.MusicSlides == nil {
		config.MusicSlides = []SlideConfig{}
	}
	if config.AudiobookSlides == nil {
		config.AudiobookSlides = []SlideConfig{}
	}

	return config
}

func saveAppConfig(config *AppConfig) {
	dir := filepath.Dir(getAppConfigPath())
	os.MkdirAll(dir, 0755)

	data, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(getAppConfigPath(), data, 0644)
}

// [LeChenMusic-END:app-management]
