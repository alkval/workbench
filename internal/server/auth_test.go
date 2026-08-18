package server

import (
	"net/http/httptest"
	"testing"
)

func TestSessionLifecycle(t *testing.T) {
	sessions := newSessionStore("a-very-long-password", false)
	response := httptest.NewRecorder()
	if !sessions.login(response, "a-very-long-password") {
		t.Fatal("login failed")
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("got %d cookies", len(cookies))
	}
	request := httptest.NewRequest("GET", "http://localhost/api/session", nil)
	request.AddCookie(cookies[0])
	if !sessions.authenticated(request) {
		t.Fatal("session should be authenticated")
	}
	logout := httptest.NewRecorder()
	sessions.logout(logout, request)
	if sessions.authenticated(request) {
		t.Fatal("session should be removed")
	}
}
