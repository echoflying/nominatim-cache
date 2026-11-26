package geocode

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNodeCacheKey(t *testing.T) {
	for _, tc := range []struct {
		point Point
		want  string
	}{
		{Point{Lat: 30.28746, Lon: 120.16145}, "30.2875_120.1615"},
		{Point{Lat: 12.34505, Lon: -12.34505}, "12.3451_-12.3451"},
		{Point{Lat: 0.03125, Lon: -0.03125}, "0.0313_-0.0313"},
		{Point{Lat: math.Copysign(0, -1), Lon: -0.00001}, "0.0000_-0.0000"},
	} {
		if got := tc.point.Key(); got != tc.want {
			t.Errorf("%+v: key = %q, want %q", tc.point, got, tc.want)
		}
	}
}

func TestNodePlaceSelection(t *testing.T) {
	cases := []struct {
		raw, name, quality, field string
	}{
		{`{"name":"朝晖中学","category":"amenity","type":"school","display_name":"朝晖中学, 文昌巷","address":{"road":"文昌巷"}}`,
			"朝晖中学", "precise", "name"},
		{`{"name":"潮王桥","category":"place","display_name":"潮王桥, 朝晖街道","address":{"suburb":"朝晖街道"}}`,
			"朝晖街道", "precise", "address.suburb"},
		{`{"display_name":"浙江省, 中国","address":{"state":"浙江省"}}`, "", "coarse", "address.state"},
		{`{"display_name":"皇史宬, 北京市","address":{}}`, "皇史宬", "regional", "display_name"},
		{`{"display_name":"","address":{}}`, "", "none", "none"},
	}
	for _, tc := range cases {
		raw, err := DecodeResponse([]byte(tc.raw))
		if err != nil {
			t.Fatal(err)
		}
		p := SelectPlace(raw)
		if p.Name != tc.name || p.Quality != tc.quality || p.SourceField != tc.field {
			t.Errorf("%s: %+v", tc.raw, p)
		}
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestUpstreamCompareSharesCooldownWithBatch(t *testing.T) {
	var requests atomic.Int32
	interactive := SourceConfig{Name: "interactive", URL: "https://example.test/interactive"}
	batch := SourceConfig{Name: "batch", URL: "https://example.test/batch"}
	upstream, err := NewUpstream([]SourceConfig{interactive}, &batch, time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	upstream.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		return &http.Response{StatusCode: http.StatusTooManyRequests,
			Header: http.Header{"Retry-After": {"10"}}, Body: http.NoBody}, nil
	})
	point := Point{Lat: 30.28746, Lon: 120.16145}
	results := upstream.Compare(t.Context(), point)
	if len(results) != 1 || results[0].Success {
		t.Fatalf("unexpected comparison: %+v", results)
	}
	_, _, err = upstream.Batch(t.Context(), point)
	var later *RetryLater
	if !errors.As(err, &later) || requests.Load() != 1 {
		t.Fatalf("batch ignored upstream cooldown: %v, calls=%d", err, requests.Load())
	}
}

func TestPhotonResult(t *testing.T) {
	const raw = ` {"type":"FeatureCollection","features":[{"properties":{"name":"朝晖文化公园","city":"杭州市","state":"浙江省","country":"中国"},"geometry":{"type":"Point","coordinates":[120.16,30.28]}}]} `
	upstream, err := NewUpstream([]SourceConfig{{Name: "photon.komoot.io", URL: "https://example.test/photon", Photon: true}}, nil, time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	upstream.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Has("format") {
			t.Error("Photon should receive coordinates without Nominatim options")
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(raw))}, nil
	})
	result, payload, err := upstream.Reverse(context.Background(), Point{Lat: 30.28746, Lon: 120.16145})
	if err != nil || result.Text("display_name") != "中国, 浙江省, 杭州市, 朝晖文化公园" || SelectPlace(result).Name != "朝晖文化公园" {
		t.Fatalf("unexpected Photon result: %+v, %v", result, err)
	}
	if payload.Source != "photon.komoot.io" || string(payload.Body) != raw {
		t.Fatalf("unexpected raw Photon response: %+v", payload)
	}
}

func TestArchivesRejectedAndAcceptedUpstreamAttempts(t *testing.T) {
	upstream, err := NewUpstream([]SourceConfig{
		{Name: "rejected", URL: "https://rejected.example.test/reverse"},
		{Name: "accepted", URL: "https://accepted.example.test/reverse"},
	}, nil, time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	upstream.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "rejected.example.test" {
			return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("gateway error"))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"display_name":"备用源"}`))}, nil
	})
	var archive []UpstreamAttempt
	upstream.Archive = func(_ context.Context, attempt UpstreamAttempt) (int64, error) {
		archive = append(archive, attempt)
		return int64(len(archive)), nil
	}
	point := Point{Lat: 1, Lon: 2}
	_, payload, err := upstream.Reverse(t.Context(), point)
	if err != nil || len(archive) != 2 || payload.ArchiveID != 2 {
		t.Fatalf("reverse error %v, archived %d attempts", err, len(archive))
	}
	if attempt := archive[0]; attempt.Source != "rejected" || attempt.Origin != "reverse" || attempt.Point != point ||
		attempt.Status != 502 || string(attempt.Body) != "gateway error" || attempt.Error == "" {
		t.Fatalf("unexpected failed attempt: %+v", attempt)
	}
	if attempt := archive[1]; attempt.Source != "accepted" || attempt.Status != 200 || attempt.Error != "" ||
		string(attempt.Body) != `{"display_name":"备用源"}` {
		t.Fatalf("unexpected accepted attempt: %+v", attempt)
	}
}

func TestReverseUsesReadyBackupWhenFirstSourceIsBusy(t *testing.T) {
	upstream, err := NewUpstream([]SourceConfig{
		{Name: "busy", URL: "https://busy.example.test/reverse"},
		{Name: "ready", URL: "https://ready.example.test/reverse"},
	}, nil, time.Second, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	upstream.sources[0].gate.busy <- struct{}{}
	defer func() { <-upstream.sources[0].gate.busy }()
	upstream.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"display_name":"备用源"}`))}, nil
	})
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	result, _, err := upstream.Reverse(ctx, Point{Lat: 1, Lon: 2})
	if err != nil || result.Text("_source") != "ready" {
		t.Fatalf("available backup was not used: %+v, %v", result, err)
	}
}

func TestReverseReservesTimeForFallback(t *testing.T) {
	upstream, err := NewUpstream([]SourceConfig{
		{Name: "slow", URL: "https://slow.example.test/reverse"},
		{Name: "ready", URL: "https://ready.example.test/reverse"},
	}, nil, time.Second, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	upstream.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "slow.example.test" {
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"display_name":"备用源"}`))}, nil
	})
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	result, payload, err := upstream.Reverse(ctx, Point{Lat: 1, Lon: 2})
	if err != nil || result.Text("_source") != "ready" {
		t.Fatalf("first upstream consumed the request budget: %+v, %v", result, err)
	}
	if payload.Source != "ready" || string(payload.Body) != `{"display_name":"备用源"}` {
		t.Fatalf("unexpected backup payload: %+v", payload)
	}
}

func TestBatchDistinguishesMissingRouteFromNoResult(t *testing.T) {
	for _, tc := range []struct {
		body, name string
		wantMiss   bool
	}{
		{"<html>404 Not Found</html>", "proxy route", false},
		{`{"error":"Not Found"}`, "JSON route", false},
		{`{"error":"Unable to geocode"}`, "no address", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			batch := SourceConfig{Name: "batch", URL: "https://batch.example.test/reverse"}
			upstream, err := NewUpstream(nil, &batch, time.Second, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			upstream.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			_, _, err = upstream.Batch(t.Context(), Point{Lat: 1, Lon: 2})
			if err == nil || errors.Is(err, ErrNoResult) != tc.wantMiss {
				t.Fatalf("unexpected error classification: %v", err)
			}
		})
	}
}
