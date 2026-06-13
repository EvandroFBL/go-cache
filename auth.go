package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"
)

const (
	// CookieName is the name of the cookie that identifies a user.
	CookieName = "cache_session"

	// CookieMaxAge is how long the cookie lasts (90 days).
	CookieMaxAge = 90 * 24 * time.Hour
)

// GenerateUserID creates a random 16-byte hex string (32 chars).
func GenerateUserID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// GetOrCreateUserID extracts the user ID from the request cookie.
// If no cookie exists, a new user ID is generated and the cookie is set.
func GetOrCreateUserID(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie(CookieName)
	if err == nil && len(cookie.Value) == 32 {
		return cookie.Value
	}

	// No valid cookie — create new user
	userID := GenerateUserID()
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    userID,
		Path:     "/",
		MaxAge:   int(CookieMaxAge.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return userID
}

// namespacedKey prefixes a key with the user ID to isolate per-user data.
// Format: "user:{userID}:{key}"
func namespacedKey(userID, key string) string {
	return "user:" + userID + ":" + key
}
