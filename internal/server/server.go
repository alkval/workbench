package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alkval/workbench/internal/process"
	"github.com/alkval/workbench/internal/store"
	systeminfo "github.com/alkval/workbench/internal/system"
)

type Server struct {
	manager  *process.Manager
	store    *store.Store
	sessions *sessionStore
	static   fs.FS
	logger   *slog.Logger
}

func New(manager *process.Manager, eventStore *store.Store, static fs.FS, password string, secureCookie bool, logger *slog.Logger) *Server {
	return &Server{
		manager: manager, store: eventStore, static: static, logger: logger,
		sessions: newSessionStore(password, secureCookie),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, map[string]bool{"ok": true}) })
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.logout)
	mux.HandleFunc("GET /api/session", s.session)
	mux.Handle("GET /api/services", s.authorize(http.HandlerFunc(s.services)))
	mux.Handle("POST /api/services/{id}/{action}", s.authorize(s.mutation(http.HandlerFunc(s.serviceAction))))
	mux.Handle("GET /api/services/{id}/logs", s.authorize(http.HandlerFunc(s.logs)))
	mux.Handle("POST /api/groups/{id}/start", s.authorize(s.mutation(http.HandlerFunc(s.startGroup))))
	mux.Handle("POST /api/stop-all", s.authorize(s.mutation(http.HandlerFunc(s.stopAll))))
	mux.Handle("GET /api/system", s.authorize(http.HandlerFunc(s.system)))
	mux.Handle("GET /api/activity", s.authorize(http.HandlerFunc(s.activity)))
	mux.Handle("GET /api/events", s.authorize(http.HandlerFunc(s.events)))
	mux.Handle("/", s.spa())
	return s.securityHeaders(s.recoverPanics(mux))
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if !s.sessions.login(w, body.Password) {
		time.Sleep(350 * time.Millisecond)
		writeError(w, http.StatusUnauthorized, "incorrect password")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.sessions.logout(w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": s.sessions.authenticated(r)})
}

func (s *Server) services(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"services": s.manager.Statuses(r.Context()), "groups": s.manager.Groups()})
}

func (s *Server) serviceAction(w http.ResponseWriter, r *http.Request) {
	id, action := r.PathValue("id"), r.PathValue("action")
	var err error
	switch action {
	case "start":
		err = s.manager.Start(r.Context(), id)
	case "stop":
		err = s.manager.Stop(r.Context(), id)
	case "restart":
		err = s.manager.Restart(r.Context(), id)
	default:
		writeError(w, http.StatusNotFound, "unknown action")
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, s.manager.Status(r.Context(), id))
}

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	logs, err := s.manager.Logs(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": logs})
}

func (s *Server) startGroup(w http.ResponseWriter, r *http.Request) {
	if err := s.manager.StartGroup(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"accepted": true})
}

func (s *Server) stopAll(w http.ResponseWriter, r *http.Request) {
	if err := s.manager.StopAll(r.Context()); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"accepted": true})
}

func (s *Server) system(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, systeminfo.Collect(r.Context()))
}

func (s *Server) activity(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	events, err := s.store.Recent(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read activity")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "stream unavailable")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	fmt.Fprint(w, "event: refresh\ndata: {}\n\n")
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprint(w, "event: refresh\ndata: {}\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) authorize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.sessions.authenticated(r) {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) mutation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Workbench-Request") != "1" {
			writeError(w, http.StatusForbidden, "request marker missing")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) spa() http.Handler {
	files := http.FileServer(http.FS(s.static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(s.static, path); err != nil {
			r.URL.Path = "/"
			path = "index.html"
		}
		if path == "index.html" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("request panic", "error", recovered)
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
