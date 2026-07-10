package nativeapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// [LeChenMusic-START:error-logs]

type ErrorLog struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Device    string `json:"device"`
	OS        string `json:"os"`
	AppVer    string `json:"appVersion"`
	Level     string `json:"level"` // error / crash / warning
	Message   string `json:"message"`
	Stack     string `json:"stack"`
	Screen    string `json:"screen"` // 页面名称
	UserID    string `json:"userId"`
}

// In-memory store (last 500 logs, survives process lifetime only)
var errorLogStore []ErrorLog
const maxErrorLogs = 500

func (api *Router) addErrorLogRoute(r chi.Router) {
	r.Route("/error-log", func(r chi.Router) {
		r.Post("/", submitErrorLog)
		r.Get("/", getErrorLogs)
		r.Delete("/", clearErrorLogs)
	})
}

func submitErrorLog(w http.ResponseWriter, r *http.Request) {
	var logEntry ErrorLog
	if err := json.NewDecoder(r.Body).Decode(&logEntry); err != nil {
		http.Error(w, "invalid request", 400)
		return
	}
	if logEntry.Timestamp == "" {
		logEntry.Timestamp = time.Now().Format(time.RFC3339)
	}
	// Auto-generate ID
	logEntry.ID = time.Now().Format("20060102150405.000")

	// Prepend to store (newest first)
	errorLogStore = append([]ErrorLog{logEntry}, errorLogStore...)
	if len(errorLogStore) > maxErrorLogs {
		errorLogStore = errorLogStore[:maxErrorLogs]
	}

	writeJSON(w, map[string]any{"status": "ok"})
}

func getErrorLogs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"data": errorLogStore, "total": len(errorLogStore)})
}

func clearErrorLogs(w http.ResponseWriter, r *http.Request) {
	errorLogStore = nil
	writeJSON(w, map[string]any{"status": "ok"})
}

// [LeChenMusic-END:error-logs]
