package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCookieIsolation verifies that two different users cannot see each other's keys.
func TestCookieIsolation(t *testing.T) {
	store := NewStore(0)
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// User A sets a key
	reqA := httptest.NewRequest("PUT", "/cache/secret", strings.NewReader(`{"value":"user-a-data","ttl":"5m"}`))
	reqA.Header.Set("Content-Type", "application/json")
	wA := httptest.NewRecorder()
	mux.ServeHTTP(wA, reqA)

	if wA.Code != http.StatusCreated {
		t.Fatalf("user A set: expected 201, got %d", wA.Code)
	}

	// Extract User A's cookie
	cookiesA := wA.Result().Cookies()
	var cookieA *http.Cookie
	for _, c := range cookiesA {
		if c.Name == CookieName {
			cookieA = c
			break
		}
	}
	if cookieA == nil {
		t.Fatal("expected cookie to be set for user A")
	}

	// User B (no cookie) should NOT see User A's keys
	reqB := httptest.NewRequest("GET", "/cache", nil)
	wB := httptest.NewRecorder()
	mux.ServeHTTP(wB, reqB)

	if wB.Code != http.StatusOK {
		t.Fatalf("user B list: expected 200, got %d", wB.Code)
	}
	body := wB.Body.String()
	if strings.Contains(body, "secret") {
		t.Fatal("user B should NOT see user A's keys")
	}

	// User B (no cookie) should NOT get User A's key
	reqB2 := httptest.NewRequest("GET", "/cache/secret", nil)
	wB2 := httptest.NewRecorder()
	mux.ServeHTTP(wB2, reqB2)

	if wB2.Code != http.StatusNotFound {
		t.Fatalf("user B get: expected 404, got %d", wB2.Code)
	}

	// User A should see their own key
	reqA2 := httptest.NewRequest("GET", "/cache/secret", nil)
	reqA2.AddCookie(cookieA)
	wA2 := httptest.NewRecorder()
	mux.ServeHTTP(wA2, reqA2)

	if wA2.Code != http.StatusOK {
		t.Fatalf("user A get: expected 200, got %d", wA2.Code)
	}
	if !strings.Contains(wA2.Body.String(), "user-a-data") {
		t.Fatal("user A should see their own data")
	}
}

// TestCookiePersistence verifies that the same cookie gets the same keys back.
func TestCookiePersistence(t *testing.T) {
	store := NewStore(0)
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// First request — set a key, get a cookie
	req1 := httptest.NewRequest("PUT", "/cache/mykey", strings.NewReader(`{"value":"persist-me","ttl":"5m"}`))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	mux.ServeHTTP(w1, req1)

	cookie := w1.Result().Cookies()[0]

	// Second request with same cookie — should find the key
	req2 := httptest.NewRequest("GET", "/cache/mykey", nil)
	req2.AddCookie(cookie)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w2.Code)
	}
	if !strings.Contains(w2.Body.String(), "persist-me") {
		t.Fatal("expected to find persisted value")
	}

	// List keys with same cookie
	req3 := httptest.NewRequest("GET", "/cache", nil)
	req3.AddCookie(cookie)
	w3 := httptest.NewRecorder()
	mux.ServeHTTP(w3, req3)

	if !strings.Contains(w3.Body.String(), "mykey") {
		t.Fatal("expected 'mykey' in key list")
	}
}

// TestCookieDeleteIsolation verifies that deleting only affects the current user.
func TestCookieDeleteIsolation(t *testing.T) {
	store := NewStore(0)
	handler := NewHandler(store)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// User A sets "shared-name"
	reqA := httptest.NewRequest("PUT", "/cache/shared-name", strings.NewReader(`{"value":"a-value","ttl":"5m"}`))
	reqA.Header.Set("Content-Type", "application/json")
	wA := httptest.NewRecorder()
	mux.ServeHTTP(wA, reqA)
	cookieA := wA.Result().Cookies()[0]

	// User B sets "shared-name" (same key name, different user)
	reqB := httptest.NewRequest("PUT", "/cache/shared-name", strings.NewReader(`{"value":"b-value","ttl":"5m"}`))
	reqB.Header.Set("Content-Type", "application/json")
	wB := httptest.NewRecorder()
	mux.ServeHTTP(wB, reqB)
	cookieB := wB.Result().Cookies()[0]

	// User A deletes their key
	reqDel := httptest.NewRequest("DELETE", "/cache/shared-name", nil)
	reqDel.AddCookie(cookieA)
	wDel := httptest.NewRecorder()
	mux.ServeHTTP(wDel, reqDel)

	if wDel.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d", wDel.Code)
	}

	// User B's key should still exist
	reqBGet := httptest.NewRequest("GET", "/cache/shared-name", nil)
	reqBGet.AddCookie(cookieB)
	wBGet := httptest.NewRecorder()
	mux.ServeHTTP(wBGet, reqBGet)

	if wBGet.Code != http.StatusOK {
		t.Fatalf("user B get after A's delete: expected 200, got %d", wBGet.Code)
	}
	if !strings.Contains(wBGet.Body.String(), "b-value") {
		t.Fatal("user B's data should survive user A's delete")
	}
}

// TestNamespacedKey verifies the key format.
func TestNamespacedKey(t *testing.T) {
	result := namespacedKey("abc123", "mykey")
	expected := "user:abc123:mykey"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

// TestGenerateUserID verifies user ID format.
func TestGenerateUserID(t *testing.T) {
	id1 := GenerateUserID()
	id2 := GenerateUserID()

	if len(id1) != 32 {
		t.Fatalf("expected 32 char ID, got %d chars: %s", len(id1), id1)
	}
	if id1 == id2 {
		t.Fatal("expected unique IDs")
	}
}
