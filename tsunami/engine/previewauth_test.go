// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testPreviewToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func newPreviewAuthTestHandler(t *testing.T, token string) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	newHTTPHandlers(makeClient()).registerHandlers(mux, handlerOpts{})
	return wrapListenHandler(mux, "localhost:0", token)
}

func serve(handler http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

var previewAPIPaths = []string{
	"/api/render", "/api/updates", "/api/data", "/api/config", "/api/schemas",
	"/api/manifest", "/api/modalresult", "/api/terminput",
}

func TestPreviewAuthRefusesEveryAPIRouteWithoutCredentials(t *testing.T) {
	handler := newPreviewAuthTestHandler(t, testPreviewToken)
	for _, path := range previewAPIPaths {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			rec := serve(handler, httptest.NewRequest(method, "http://localhost:1234"+path, strings.NewReader("{}")))
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s without credentials: status %d, want 401", method, path, rec.Code)
			}
		}
	}
}

func TestPreviewAuthRefusesWrongCredentials(t *testing.T) {
	handler := newPreviewAuthTestHandler(t, testPreviewToken)
	cases := map[string]func(*http.Request){
		"wrong cookie": func(r *http.Request) { r.AddCookie(&http.Cookie{Name: previewAuthCookieName, Value: "nope"}) },
		"empty cookie": func(r *http.Request) { r.AddCookie(&http.Cookie{Name: previewAuthCookieName, Value: ""}) },
		"prefix of token": func(r *http.Request) {
			r.AddCookie(&http.Cookie{Name: previewAuthCookieName, Value: testPreviewToken[:10]})
		},
		"token as query":    func(r *http.Request) { r.URL.RawQuery = "token=" + testPreviewToken },
		"wrong bearer":      func(r *http.Request) { r.Header.Set("Authorization", "Bearer nope") },
		"bare token header": func(r *http.Request) { r.Header.Set("Authorization", testPreviewToken) },
		"basic scheme":      func(r *http.Request) { r.Header.Set("Authorization", "Basic "+testPreviewToken) },
		"other cookie name": func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "session", Value: testPreviewToken}) },
	}
	for name, mutate := range cases {
		req := httptest.NewRequest(http.MethodGet, "http://localhost:1234/api/data", nil)
		mutate(req)
		if rec := serve(handler, req); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status %d, want 401", name, rec.Code)
		}
	}
}

func TestPreviewAuthAcceptsCookieAndBearer(t *testing.T) {
	handler := newPreviewAuthTestHandler(t, testPreviewToken)
	for _, path := range previewAPIPaths {
		withCookie := httptest.NewRequest(http.MethodGet, "http://localhost:1234"+path, nil)
		withCookie.AddCookie(&http.Cookie{Name: previewAuthCookieName, Value: testPreviewToken})
		if rec := serve(handler, withCookie); rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
			t.Errorf("GET %s with the cookie: status %d", path, rec.Code)
		}
		withBearer := httptest.NewRequest(http.MethodGet, "http://localhost:1234"+path, nil)
		withBearer.Header.Set("Authorization", "Bearer "+testPreviewToken)
		if rec := serve(handler, withBearer); rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
			t.Errorf("GET %s with a bearer token: status %d", path, rec.Code)
		}
	}
}

func TestPreviewAuthURLTokenSetsCookieAndStripsItself(t *testing.T) {
	handler := newPreviewAuthTestHandler(t, testPreviewToken)
	req := httptest.NewRequest(http.MethodGet, "http://localhost:1234/?clientid=wave:b1&"+previewAuthQueryParam+"="+testPreviewToken, nil)
	rec := serve(handler, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d, want 303", rec.Code)
	}
	location := rec.Header().Get("Location")
	if strings.Contains(location, testPreviewToken) || strings.Contains(location, previewAuthQueryParam) {
		t.Fatalf("redirect still carries the token: %q", location)
	}
	if location != "/?clientid=wave%3Ab1" && location != "/?clientid=wave:b1" {
		t.Fatalf("redirect %q dropped the other query parameters", location)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("%d cookies set, want 1", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != previewAuthCookieName || cookie.Value != testPreviewToken {
		t.Errorf("cookie %s=%q", cookie.Name, cookie.Value)
	}
	if !cookie.HttpOnly {
		t.Error("cookie is readable from page script")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite %v, want Strict", cookie.SameSite)
	}
	if cookie.Path != "/" {
		t.Errorf("cookie path %q", cookie.Path)
	}

	follow := httptest.NewRequest(http.MethodGet, "http://localhost:1234/api/data", nil)
	follow.AddCookie(cookie)
	if rec := serve(handler, follow); rec.Code == http.StatusUnauthorized {
		t.Fatal("the cookie the server set does not authorise the API")
	}
}

func TestPreviewAuthWrongURLTokenSetsNoCookie(t *testing.T) {
	handler := newPreviewAuthTestHandler(t, testPreviewToken)
	req := httptest.NewRequest(http.MethodGet, "http://localhost:1234/?"+previewAuthQueryParam+"=wrong", nil)
	rec := serve(handler, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want 401", rec.Code)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("a wrong token set a cookie")
	}
}

func TestPreviewAuthLeavesPagesAndAssetsOpen(t *testing.T) {
	handler := newPreviewAuthTestHandler(t, testPreviewToken)
	for _, path := range []string{"/", "/static/logo.png", "/dyn/x"} {
		rec := serve(handler, httptest.NewRequest(http.MethodGet, "http://localhost:1234"+path, nil))
		if rec.Code == http.StatusUnauthorized {
			t.Errorf("GET %s needs credentials, but only /api/* should", path)
		}
	}
}

func TestPreviewAuthDisabledWithoutAToken(t *testing.T) {
	handler := newPreviewAuthTestHandler(t, "")
	rec := serve(handler, httptest.NewRequest(http.MethodGet, "http://localhost:1234/api/data", nil))
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("an app started without a token demanded one: status %d", rec.Code)
	}
}

func TestPreviewAuthKeepsLoopbackHostGuardOutermost(t *testing.T) {
	handler := newPreviewAuthTestHandler(t, testPreviewToken)
	req := httptest.NewRequest(http.MethodGet, "http://localhost:1234/api/data", nil)
	req.Host = "evil.example:1234"
	req.AddCookie(&http.Cookie{Name: previewAuthCookieName, Value: testPreviewToken})
	if rec := serve(handler, req); rec.Code != http.StatusForbidden {
		t.Fatalf("a rebinding Host with a valid cookie got status %d, want 403", rec.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "http://localhost:1234/?"+previewAuthQueryParam+"="+testPreviewToken, nil)
	req.Host = "evil.example:1234"
	if rec := serve(handler, req); rec.Code != http.StatusForbidden || len(rec.Result().Cookies()) != 0 {
		t.Fatalf("a rebinding Host exchanged the URL token: status %d", rec.Code)
	}
}

func TestPreviewAuthPreflightIsNotChallenged(t *testing.T) {
	handler := newPreviewAuthTestHandler(t, testPreviewToken)
	rec := serve(handler, httptest.NewRequest(http.MethodOptions, "http://localhost:1234/api/data", nil))
	if rec.Code == http.StatusUnauthorized {
		t.Fatal("a credential-less preflight was challenged")
	}
}
