// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"crypto/subtle"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// The builder starts each preview run with a random token in this variable. The port is on
// loopback, which any local process or page can reach; the token keeps them out of /api/*.
// An app started without it (the normal Wave block flow) is not affected.
const TsunamiAuthTokenEnvVar = "TSUNAMI_AUTHTOKEN"

const (
	previewAuthCookieName = "tsunami_auth"
	previewAuthQueryParam = "tsunamitoken"
	previewAuthBearer     = "Bearer "
)

func tokensEqual(got string, want string) bool {
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func hasPreviewCredentials(r *http.Request, token string) bool {
	if cookie, err := r.Cookie(previewAuthCookieName); err == nil && tokensEqual(cookie.Value, token) {
		return true
	}
	auth := r.Header.Get("Authorization")
	return strings.HasPrefix(auth, previewAuthBearer) && tokensEqual(strings.TrimPrefix(auth, previewAuthBearer), token)
}

// The preview URL carries the token once: the first page load exchanges it for an HttpOnly
// SameSite=Strict cookie and is redirected to the same URL without it, so the token is not
// left in the page's address, history or Referer. Same-origin fetches and the EventSource of
// the built app then carry the cookie by themselves, so app authors change nothing.
func previewAuthGuard(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if given := r.URL.Query().Get(previewAuthQueryParam); given != "" {
			if !tokensEqual(given, token) {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:     previewAuthCookieName,
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteStrictMode,
			})
			http.Redirect(w, r, withoutPreviewToken(r.URL), http.StatusSeeOther)
			return
		}
		// A preflight carries no credentials and no data.
		if r.Method != http.MethodOptions && strings.HasPrefix(r.URL.Path, "/api/") && !hasPreviewCredentials(r, token) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withoutPreviewToken(u *url.URL) string {
	query := u.Query()
	query.Del(previewAuthQueryParam)
	rel := url.URL{Path: u.Path, RawQuery: query.Encode()}
	if rel.Path == "" {
		rel.Path = "/"
	}
	return rel.String()
}

func wrapListenHandler(mux http.Handler, listenAddr string, token string) http.Handler {
	handler := previewAuthGuard(token, mux)
	if listenHost, _, err := net.SplitHostPort(listenAddr); err == nil && isLoopbackHostname(listenHost) {
		handler = loopbackHostGuard(handler)
	}
	return handler
}
