package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	appanalysis "connect-to-mongodb/internal/analysis"
	appLogger "connect-to-mongodb/internal/logger"
	"github.com/joho/godotenv"

	httpSwagger "github.com/swaggo/http-swagger"
	_ "connect-to-mongodb/docs"


)

//@title Connect to MongoDB API
//@version 1.0
//@description This is a sample server for Connect to MongoDB API.
//@host modul-go.onrender.com
//@BasePath /
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

//@Summary Write JSON response
//@Description Write JSON response
//@Tags analysis
//@Accept json
//@Produce json
//@Success 200 {object} appanalysis.AnalysisReport
func dummyHealthz() {}

//@Summary Log HTTP requests
//@Description Logs HTTP requests for monitoring and debugging
//@Tags logging
//@Accept json
//@Produce json
//@Success 200 {object} map[string]string
//@Failure 500 {object} map[string]string
//@Router /ws/logs [get]
func dummyRequestLogger() {}

//@Summary Analyze user activities
//@Description Analyzes user activities and returns a report
//@Tags analysis
//@Accept json
//@Produce json
//@Param login query string true "User login"
//@Success 200 {object} appanalysis.AnalysisReport
//@Failure 400 {object} map[string]string
//@Failure 502 {object} map[string]string
//@Router /analyze [get]
func dummyAnalyze() {}

//@Summary Generate JSON report
//@Description Generates a JSON report of all user activities
//@Tags analysis
//@Produce json
//@Success 200 {array} appanalysis.AnalysisReport
//@Failure 500 {object} map[string]string
//@Router /report [get]
func dummyReport() {}
func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func requestLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("HTTP request", "method", r.Method, "path", r.URL.Path, "remote_addr", r.RemoteAddr, "duration_ms", time.Since(started).Milliseconds())
	})
}
