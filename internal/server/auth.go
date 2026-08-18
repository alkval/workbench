package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"sync"
	"time"
)

const sessionCookie = "workbench_session"

type sessionStore struct {
	mu       sync.Mutex
	tokens   map[string]time.Time
	password string
	secure   bool
}

func newSessionStore(password string, secure bool) *sessionStore {
	return &sessionStore{tokens: make(map[string]time.Time), password: password, secure: secure}
}

func (s *sessionStore) login(response http.ResponseWriter, password string) bool {
	if subtle.ConstantTimeCompare([]byte(password), []byte(s.password)) != 1 {
		return false
	}
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return false
	}
	token := base64.RawURLEncoding.EncodeToString(bytes)
	expires := time.Now().Add(12 * time.Hour)
	s.mu.Lock()
	s.tokens[token] = expires
	s.pruneLocked()
	s.mu.Unlock()
	http.SetCookie(response, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", Expires: expires,
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode,
	})
	return true
}

func (s *sessionStore) authenticated(request *http.Request) bool {
	cookie, err := request.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	expires, ok := s.tokens[cookie.Value]
	if !ok || time.Now().After(expires) {
		delete(s.tokens, cookie.Value)
		return false
	}
	return true
}

func (s *sessionStore) logout(response http.ResponseWriter, request *http.Request) {
	if cookie, err := request.Cookie(sessionCookie); err == nil {
		s.mu.Lock()
		delete(s.tokens, cookie.Value)
		s.mu.Unlock()
	}
	http.SetCookie(response, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteStrictMode,
	})
}

func (s *sessionStore) pruneLocked() {
	now := time.Now()
	for token, expires := range s.tokens {
		if now.After(expires) {
			delete(s.tokens, token)
		}
	}
}
