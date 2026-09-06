package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Google ID-token (JWT) verification for /v1/auth and /v1/sync routes.
// Tokens are verified via the tokeninfo endpoint; results are cached by
// token hash until exp so Google is not hit on every request.

const googleTokenInfoURL = "https://oauth2.googleapis.com/tokeninfo?id_token="

type userInfo struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
}

type tokenInfoResponse struct {
	Iss   string `json:"iss"`
	Aud   string `json:"aud"`
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Exp   int64  `json:"exp,string"`
}

type cachedToken struct {
	user userInfo
	exp  time.Time
}

type googleAuth struct {
	clientID   string
	allowDev   bool // ACCEPT_DEV_TOKENS: accept "dev:<user_id>" tokens, local testing only
	httpClient *http.Client

	mu     sync.Mutex
	tokens map[[32]byte]cachedToken
}

func newGoogleAuth(clientID string, allowDev bool) *googleAuth {
	return &googleAuth{
		clientID:   clientID,
		allowDev:   allowDev,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		tokens:     make(map[[32]byte]cachedToken),
	}
}

// verifyToken checks a Google ID-token and returns the user identity.
func (g *googleAuth) verifyToken(ctx context.Context, token string) (userInfo, error) {
	if g.allowDev {
		if sub, ok := strings.CutPrefix(token, "dev:"); ok && sub != "" {
			return userInfo{Sub: sub, Email: sub + "@dev.invalid"}, nil
		}
	}
	sum := sha256.Sum256([]byte(token))

	g.mu.Lock()
	if c, ok := g.tokens[sum]; ok {
		g.mu.Unlock()
		if time.Now().Before(c.exp) {
			return c.user, nil
		}
	} else {
		g.mu.Unlock()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleTokenInfoURL+token, nil)
	if err != nil {
		return userInfo{}, err
	}
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return userInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return userInfo{}, errors.New("tokeninfo returned " + resp.Status)
	}

	var info tokenInfoResponse
	if err := json.NewDecoder(http.MaxBytesReader(nil, resp.Body, maxBodyBytes)).Decode(&info); err != nil {
		return userInfo{}, err
	}
	if info.Sub == "" {
		return userInfo{}, errors.New("tokeninfo response has no sub")
	}
	if info.Iss != "accounts.google.com" && info.Iss != "https://accounts.google.com" {
		return userInfo{}, errors.New("unexpected iss: " + info.Iss)
	}
	if info.Aud != g.clientID {
		return userInfo{}, errors.New("aud does not match GOOGLE_CLIENT_ID")
	}
	if time.Now().Unix() >= info.Exp {
		return userInfo{}, errors.New("token expired")
	}

	user := userInfo{Sub: info.Sub, Email: info.Email}
	g.mu.Lock()
	// Cache slightly shorter than exp: the client refreshes tokens hourly.
	g.tokens[sum] = cachedToken{user: user, exp: time.Unix(info.Exp, 0).Add(-time.Minute)}
	// opportunistic cleanup of expired entries
	for k, v := range g.tokens {
		if time.Now().After(v.exp) {
			delete(g.tokens, k)
		}
	}
	g.mu.Unlock()
	return user, nil
}

type ctxKeyUserInfo struct{}

// bearerAuthMiddleware guards /v1/auth and /v1/sync with a Google ID-token.
func bearerAuthMiddleware(next http.Handler, auth *googleAuth) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
			return
		}
		user, err := auth.verifyToken(r.Context(), token)
		if err != nil {
			log.Printf("google auth failed: %v", err)
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "unauthorized"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKeyUserInfo{}, user)))
	})
}

func userFromContext(r *http.Request) userInfo {
	user, _ := r.Context().Value(ctxKeyUserInfo{}).(userInfo)
	return user
}

// handleWhoami echoes the verified identity back to the app.
func handleWhoami(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r)
	writeJSON(w, http.StatusOK, map[string]string{"user_id": user.Sub, "email": user.Email})
}
