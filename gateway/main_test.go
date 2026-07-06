package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestLoggerCallsHandler(t *testing.T) {
	called := false
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := requestLogger(logger, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(http.StatusNoContent) }))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if !called || recorder.Code != http.StatusNoContent {
		t.Fatal("handler was not called correctly")
	}
}
