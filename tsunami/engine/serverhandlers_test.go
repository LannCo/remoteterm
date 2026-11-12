// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func newConfigTestClient(t *testing.T) (*ClientImpl, http.Handler) {
	t.Helper()
	c := makeClient()
	minV, maxV := 0.0, 10.0
	c.Root.RegisterAtom("$config.level", MakeAtomImpl(5, &AtomMeta{Min: &minV, Max: &maxV}))
	c.Root.RegisterAtom("$config.mode", MakeAtomImpl("fast", &AtomMeta{Enum: []string{"fast", "slow"}}))
	c.Root.RegisterAtom("$config.name", MakeAtomImpl("abc", &AtomMeta{Pattern: "^[a-z]+$"}))
	c.Root.RegisterAtom("$config.plain", MakeAtomImpl("x", nil))
	c.Root.Atoms["$config.level"].SetUsedBy("comp-level", true)
	mux := http.NewServeMux()
	newHTTPHandlers(c).registerHandlers(mux, handlerOpts{})
	return c, loopbackHostGuard(mux)
}

func postConfig(handler http.Handler, contentType string, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "http://localhost:1234/api/config", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestConfigPostValidation(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		body        string
		wantStatus  int
	}{
		{"text-plain-forgery", "text/plain", `{"plain":"y"}`, http.StatusUnsupportedMediaType},
		{"form-forgery", "application/x-www-form-urlencoded", `{"plain":"y"}`, http.StatusUnsupportedMediaType},
		{"missing-content-type", "", `{"plain":"y"}`, http.StatusUnsupportedMediaType},
		{"unknown-key", "application/json", `{"nope":1}`, http.StatusBadRequest},
		{"below-min", "application/json", `{"level":-1}`, http.StatusBadRequest},
		{"above-max", "application/json", `{"level":11}`, http.StatusBadRequest},
		{"wrong-type", "application/json", `{"level":"high"}`, http.StatusBadRequest},
		{"fractional-int", "application/json", `{"level":1.5}`, http.StatusBadRequest},
		{"not-in-enum", "application/json", `{"mode":"warp"}`, http.StatusBadRequest},
		{"pattern-mismatch", "application/json", `{"name":"ABC"}`, http.StatusBadRequest},
		{"null-reset", "application/json", `{"plain":null}`, http.StatusBadRequest},
		{"one-bad-key-rejects-all", "application/json", `{"plain":"changed","level":99}`, http.StatusBadRequest},
		{"bad-json", "application/json", `{`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, handler := newConfigTestClient(t)
			rec := postConfig(handler, tc.contentType, tc.body)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if got := c.Root.GetConfigMap(); got["level"] != 5 || got["mode"] != "fast" || got["name"] != "abc" || got["plain"] != "x" {
				t.Fatalf("rejected request modified config: %v", got)
			}
			if work := c.Root.getAndClearRenderWork(); len(work) != 0 {
				t.Fatalf("rejected request queued render work: %v", work)
			}
		})
	}
}

func TestConfigPostAppliesAndQueuesRender(t *testing.T) {
	c, handler := newConfigTestClient(t)
	rec := postConfig(handler, "application/json; charset=utf-8", `{"level":10,"mode":"slow","name":"xyz"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d; body=%s", rec.Code, rec.Body.String())
	}
	got := c.Root.GetConfigMap()
	if got["level"] != 10 || got["mode"] != "slow" || got["name"] != "xyz" {
		t.Fatalf("config not applied: %v", got)
	}
	if work := c.Root.getAndClearRenderWork(); !slices.Contains(work, "comp-level") {
		t.Fatalf("config update did not queue a render for its user; work=%v", work)
	}
}

func TestPostEndpointsRequireJSONContentType(t *testing.T) {
	c := makeClient()
	mux := http.NewServeMux()
	newHTTPHandlers(c).registerHandlers(mux, handlerOpts{})
	for _, path := range []string{"/api/render", "/api/modalresult", "/api/terminput"} {
		req := httptest.NewRequest(http.MethodPost, "http://localhost:1234"+path, strings.NewReader(`{"dispose":true,"forcetakeover":true,"clientid":"x"}`))
		req.Header.Set("Content-Type", "text/plain")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Errorf("%s accepted text/plain: status %d", path, rec.Code)
		}
	}
	if c.GetIsDone() {
		t.Fatalf("forged text/plain dispose shut the app down")
	}
}

func TestLoopbackHostGuard(t *testing.T) {
	handler := loopbackHostGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	cases := map[string]int{
		"localhost:1234":    http.StatusOK,
		"LOCALHOST:1234":    http.StatusOK,
		"127.0.0.1:1234":    http.StatusOK,
		"[::1]:1234":        http.StatusOK,
		"localhost":         http.StatusOK,
		"evil.example:1234": http.StatusForbidden,
		"192.168.1.5:1234":  http.StatusForbidden,
		"localhost.evil.io": http.StatusForbidden,
	}
	for host, want := range cases {
		req := httptest.NewRequest(http.MethodGet, "http://localhost/api/data", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("Host %q: status %d, want %d", host, rec.Code, want)
		}
	}
}
