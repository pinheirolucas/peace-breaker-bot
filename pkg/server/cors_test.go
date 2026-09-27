package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCorsMiddleware_OptionsPreflight(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	handler := corsMiddleware(next)

	req := httptest.NewRequest(http.MethodOptions, "/bot/play", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if called {
		t.Fatal("expected wrapped handler not to be called for an OPTIONS preflight")
	}

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected Access-Control-Allow-Origin %q, got %q", "*", got)
	}

	allowedMethods := rec.Header().Get("Access-Control-Allow-Methods")
	for _, method := range []string{"GET", "POST", "PUT", "OPTIONS"} {
		if !strings.Contains(allowedMethods, method) {
			t.Fatalf("expected Access-Control-Allow-Methods %q to contain %q", allowedMethods, method)
		}
	}

	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type" {
		t.Fatalf("expected Access-Control-Allow-Headers %q, got %q", "Content-Type", got)
	}
}

func TestCorsMiddleware_GetPassesThrough(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := corsMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/instant/list", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("expected wrapped handler to be called for a GET request")
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected Access-Control-Allow-Origin %q, got %q", "*", got)
	}

	allowedMethods := rec.Header().Get("Access-Control-Allow-Methods")
	for _, method := range []string{"GET", "POST", "PUT", "OPTIONS"} {
		if !strings.Contains(allowedMethods, method) {
			t.Fatalf("expected Access-Control-Allow-Methods %q to contain %q", allowedMethods, method)
		}
	}

	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type" {
		t.Fatalf("expected Access-Control-Allow-Headers %q, got %q", "Content-Type", got)
	}
}
