package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/echoflying/nominatim-cache/backend/internal/config"
)

func TestPlaceHelpAdvertisesPublicPaths(t *testing.T) {
	s := Server{Config: config.Config{BasePath: "/nominatim"}}
	handler := s.Handler()
	help := httptest.NewRecorder()
	handler.ServeHTTP(help, httptest.NewRequest(http.MethodGet, "/nominatim/help", nil))
	if help.Code != http.StatusOK || help.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("help returned %d with cache policy %q", help.Code, help.Header().Get("Cache-Control"))
	}
	var guide struct {
		Endpoints []struct{ Method, Path string }
		Example   string
		Fields    map[string]string `json:"response_fields"`
	}
	if err := json.NewDecoder(help.Body).Decode(&guide); err != nil {
		t.Fatal(err)
	}
	wanted := map[string]bool{"/nominatim/reverse": true, "/nominatim/health": true, "/nominatim/help": true}
	for _, endpoint := range guide.Endpoints {
		if endpoint.Method != http.MethodGet || !wanted[endpoint.Path] {
			t.Errorf("unexpected public endpoint: %v", endpoint)
		}
		delete(wanted, endpoint.Path)
		preflight := httptest.NewRecorder()
		handler.ServeHTTP(preflight, httptest.NewRequest(http.MethodOptions, endpoint.Path, nil))
		if preflight.Code != http.StatusNoContent || preflight.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("preflight for %s returned %d or lacked CORS", endpoint.Path, preflight.Code)
		}
	}
	if len(wanted) != 0 || guide.Example != "/nominatim/reverse?lat=30.28746&lon=120.16145" || guide.Fields["place_name"] == "" || guide.Fields["place_quality"] == "" {
		t.Fatalf("incomplete help: missing routes %v, got %+v", wanted, guide)
	}

	for _, path := range []string{"/nominatim/reverse", "/api/reverse", "/nominatim/api/reverse"} {
		reverse := httptest.NewRecorder()
		handler.ServeHTTP(reverse, httptest.NewRequest(http.MethodGet, path+"?lat=invalid&lon=120", nil))
		if reverse.Code != http.StatusBadRequest || reverse.Header().Get("Access-Control-Expose-Headers") != "Retry-After" {
			t.Errorf("reverse route %s failed coordinate validation or lacked a readable retry header (HTTP %d)", path, reverse.Code)
		}
	}
}

func TestAdminEndpointsAreNotExposedOnPublicListener(t *testing.T) {
	s := Server{Config: config.Config{BasePath: "/nominatim"}}
	for _, tc := range []struct {
		handler http.Handler
		path    string
		status  int
	}{
		{s.Handler(), "/admin/stats", http.StatusNotFound},
		{s.Handler(), "/nominatim/admin/stats", http.StatusNotFound},
		{s.Handler(), "/nominatim/", http.StatusNotFound},
		{s.AdminHandler(), "/admin/stats", http.StatusUnauthorized},
		{s.Handler(), "/admin/logs/1", http.StatusNotFound},
		{s.AdminHandler(), "/admin/logs/1", http.StatusUnauthorized},
		{s.AdminHandler(), "/api/reverse", http.StatusNotFound},
	} {
		recorder := httptest.NewRecorder()
		tc.handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if recorder.Code != tc.status {
			t.Errorf("GET %s: status %d, want %d", tc.path, recorder.Code, tc.status)
		}
	}
}

func TestAltviewBatchEntryUsesDedicatedPublicRoutes(t *testing.T) {
	handler := (Server{Config: config.Config{
		BasePath: "/nominatim",
		Tokens:   map[string]string{"test-caller": "this-is-a-long-enough-test-bearer-token"},
	}}).Handler()
	preflight := httptest.NewRecorder()
	handler.ServeHTTP(preflight, httptest.NewRequest(http.MethodOptions, "/nominatim/altview/place-batches", nil))
	if preflight.Code != http.StatusNoContent || preflight.Header().Get("Access-Control-Allow-Methods") != "POST, OPTIONS" {
		t.Fatalf("Altview batch preflight returned %d", preflight.Code)
	}

	missingKey := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/nominatim/altview/place-batches", strings.NewReader(`{"points":[{"lat":30,"lon":120}]}`))
	handler.ServeHTTP(missingKey, req)
	if missingKey.Code != http.StatusBadRequest {
		t.Fatalf("Altview batch should validate the idempotency key before storage, got %d", missingKey.Code)
	}

	privateRoute := httptest.NewRecorder()
	handler.ServeHTTP(privateRoute, httptest.NewRequest(http.MethodGet, "/nominatim/api/reverse/batches/0123456789abcdef0123456789abcdef", nil))
	if privateRoute.Code != http.StatusUnauthorized {
		t.Fatalf("general batch API became public: got %d", privateRoute.Code)
	}
}
