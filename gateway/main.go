package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	appanalysis "connect-to-mongodb/internal/analysis"
	appLogger "connect-to-mongodb/internal/logger"
	"github.com/joho/godotenv"

	_ "connect-to-mongodb/docs"
	httpSwagger "github.com/swaggo/http-swagger"
)

// @title Connect to MongoDB API
// @version 1.0
// @description This is a sample server for Connect to MongoDB API.
// @host modul-go.onrender.com
// @BasePath /
func main() {
	_ = godotenv.Load()
	logger, logHub := appLogger.New()
	slog.SetDefault(logger)
	repo, err := appanalysis.NewRepository(context.Background())
	if err != nil {
		logger.Error("failed to connect to MongoDB", "error", err)
		os.Exit(1)
	}
	defer repo.Close(context.Background())
	service := appanalysis.NewService(repo)
	mux := http.NewServeMux()
	mux.Handle("/swagger/", httpSwagger.WrapHandler)
	mux.Handle("/ws/logs", logHub)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/analyze", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		login := r.URL.Query().Get("login")
		if login == "" {
			http.Error(w, "missing login parameter", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		response, err := service.AnalyzeUser(ctx, login)
		if err != nil {
			logger.Error("analysis failed", "login", login, "error", err)
			http.Error(w, "analysis failed", http.StatusBadGateway)
			return
		}
		logger.Info("analysis completed", "login", login, "duration_minutes", response.TotalDuration, "limits_exceeded", response.ExceededLimits)
		writeJSON(w, response)
	})
	mux.HandleFunc("/report", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		results, err := service.ListAll(ctx)
		if err != nil {
			logger.Error("report generation failed", "error", err)
			http.Error(w, "report generation failed", http.StatusInternalServerError)
			return
		}
		logger.Info("JSON report generated", "users", len(results))
		w.Header().Set("Content-Disposition", `attachment; filename="activity-report.json"`)
		writeJSON(w, results)
	})
	mux.HandleFunc("/import", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload any
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 10<<20))
		if err := decoder.Decode(&payload); err != nil {
			http.Error(w, "invalid JSON file", http.StatusBadRequest)
			return
		}
		users, ok := normalizeImportedUsers(payload)
		if !ok || len(users) == 0 {
			http.Error(w, "JSON must contain a user object or an array of user objects with login", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		imported, err := service.ImportUsers(ctx, users)
		if err != nil {
			logger.Error("import failed", "error", err)
			http.Error(w, "import failed", http.StatusInternalServerError)
			return
		}
		logger.Info("JSON data imported", "users", imported)
		writeJSON(w, map[string]int{"imported": imported})
	})
	mux.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		deleted, err := service.DeleteAll(ctx)
		if err != nil {
			logger.Error("database cleanup failed", "error", err)
			http.Error(w, "database cleanup failed", http.StatusInternalServerError)
			return
		}
		logger.Info("MongoDB collection cleared", "deleted", deleted)
		writeJSON(w, map[string]int64{"deleted": deleted})
	})
	mux.HandleFunc("/users/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		login, err := url.PathUnescape(strings.TrimPrefix(r.URL.Path, "/users/"))
		if err != nil || strings.TrimSpace(login) == "" {
			http.Error(w, "invalid login", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		deleted, err := service.DeleteByLogin(ctx, login)
		if err != nil {
			logger.Error("user deletion failed", "login", login, "error", err)
			http.Error(w, "user deletion failed", http.StatusInternalServerError)
			return
		}
		logger.Info("MongoDB user delete requested", "login", login, "deleted", deleted)
		writeJSON(w, map[string]int64{"deleted": deleted})
	})
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	server := &http.Server{Addr: "0.0.0.0:" + port, Handler: requestLogger(logger, mux), ReadHeaderTimeout: 5 * time.Second}
	logger.Info("HTTP gateway started", "port", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("HTTP gateway stopped", "error", err)
		os.Exit(1)
	}
}

// @Summary Write JSON response
// @Description Write JSON response
// @Tags analysis
// @Accept json
// @Produce json
// @Success 200 {object} appanalysis.AnalysisReport
func dummyHealthz() {}

// @Summary Log HTTP requests
// @Description Logs HTTP requests for monitoring and debugging
// @Tags logging
// @Accept json
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /ws/logs [get]
func dummyRequestLogger() {}

// @Summary Analyze user activities
// @Description Analyzes user activities and returns a report
// @Tags analysis
// @Accept json
// @Produce json
// @Param login query string true "User login"
// @Success 200 {object} appanalysis.AnalysisReport
// @Failure 400 {object} map[string]string
// @Failure 502 {object} map[string]string
// @Router /analyze [get]
func dummyAnalyze() {}

// @Summary Generate JSON report
// @Description Generates a JSON report of all user activities
// @Tags analysis
// @Produce json
// @Success 200 {array} appanalysis.AnalysisReport
// @Failure 500 {object} map[string]string
// @Router /report [get]
func dummyReport() {}

// @Summary Import users from JSON
// @Description Imports users from a JSON payload
// @Tags analysis
// @Accept json
// @Produce json
// @Param payload body any true "JSON payload containing user objects"
// @Success 200 {object} map[string]int
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /import [post]
func dummyImport() {}

// @Summary Delete all users
// @Description Deletes all users from the database
// @Tags analysis
// @Produce json
// @Success 200 {object} map[string]int64
// @Failure 500 {object} map[string]string
// @Router /users [delete]
func dummyDeleteAllUsers() {}

// @Summary Delete user by login
// @Description Deletes a user by their login from the database
// @Tags analysis
// @Produce json
// @Param login path string true "User login"
// @Success 200 {object} map[string]int64
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /users/{login} [delete]
func dummyDeleteUserByLogin() {}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func normalizeImportedUsers(payload any) ([]map[string]interface{}, bool) {
	switch value := payload.(type) {
	case map[string]interface{}:
		if login, ok := value["login"].(string); ok && login != "" {
			return []map[string]interface{}{value}, true
		}
	case []interface{}:
		users := make([]map[string]interface{}, 0, len(value))
		for _, item := range value {
			user, ok := item.(map[string]interface{})
			if !ok {
				return nil, false
			}
			if login, ok := user["login"].(string); ok && login != "" {
				users = append(users, user)
			}
		}
		return users, len(users) > 0
	}
	return nil, false
}

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("HTTP request", "method", r.Method, "path", r.URL.Path, "remote_addr", r.RemoteAddr, "duration_ms", time.Since(started).Milliseconds())
	})
}
