package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/echoflying/nominatim-cache/backend/internal/config"
)

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
		{s.AdminHandler(), "/api/reverse", http.StatusNotFound},
	} {
		recorder := httptest.NewRecorder()
		tc.handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if recorder.Code != tc.status {
			t.Errorf("GET %s: status %d, want %d", tc.path, recorder.Code, tc.status)
		}
	}
}
