package process

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/alkval/workbench/internal/config"
	"github.com/alkval/workbench/internal/store"
)

type State string

const (
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StateError    State = "error"
)

type Status struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	State        State     `json:"state"`
	Managed      bool      `json:"managed"`
	Healthy      bool      `json:"healthy"`
	Port         int       `json:"port,omitempty"`
	OpenURL      string    `json:"open_url,omitempty"`
	Dependencies []string  `json:"dependencies,omitempty"`
	StartedAt    time.Time `json:"started_at,omitempty"`
	LastExitCode *int      `json:"last_exit_code,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
	CanStart     bool      `json:"can_start"`
	CanStop      bool      `json:"can_stop"`
	CanRestart   bool      `json:"can_restart"`
}

type runtime struct {
	mu           sync.Mutex
	cmd          *exec.Cmd
	startedAt    time.Time
	lastExitCode *int
	lastError    string
	starting     bool
	stopping     bool
	logs         *logBuffer
}

type Manager struct {
	services map[string]config.Service
	groups   map[string]config.Group
	runtimes map[string]*runtime
	store    *store.Store
	client   *http.Client
}

func New(cfg config.Config, eventStore *store.Store) *Manager {
	m := &Manager{
		services: make(map[string]config.Service, len(cfg.Services)),
		groups:   make(map[string]config.Group, len(cfg.Groups)),
		runtimes: make(map[string]*runtime, len(cfg.Services)),
		store:    eventStore,
		client:   &http.Client{Timeout: 1200 * time.Millisecond},
	}
	for _, svc := range cfg.Services {
		m.services[svc.ID] = svc
		m.runtimes[svc.ID] = &runtime{logs: newLogBuffer(256 * 1024)}
	}
	for _, group := range cfg.Groups {
		m.groups[group.ID] = group
	}
	return m
}

func (m *Manager) Groups() []config.Group {
	groups := make([]config.Group, 0, len(m.groups))
	for _, group := range m.groups {
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	return groups
}

func (m *Manager) Statuses(ctx context.Context) []Status {
	ids := make([]string, 0, len(m.services))
	for id := range m.services {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	statuses := make([]Status, 0, len(ids))
	for _, id := range ids {
		statuses = append(statuses, m.Status(ctx, id))
	}
	return statuses
}

func (m *Manager) Status(ctx context.Context, id string) Status {
	svc, ok := m.services[id]
	if !ok {
		return Status{ID: id, State: StateError, LastError: "unknown service"}
	}
	rt := m.runtimes[id]
	rt.mu.Lock()
	managed := rt.cmd != nil && rt.cmd.ProcessState == nil
	starting := rt.starting
	startedAt := rt.startedAt
	lastExitCode := rt.lastExitCode
	lastError := rt.lastError
	rt.mu.Unlock()
	healthy := m.healthy(ctx, svc.HealthURL)
	state := StateStopped
	if managed {
		switch {
		case starting && svc.HealthURL != "" && !healthy:
			state = StateStarting
		case lastError != "":
			state = StateError
		default:
			state = StateRunning
		}
	} else if healthy {
		state = StateRunning
	} else if lastError != "" {
		state = StateError
	}
	return Status{
		ID: id, Name: svc.Name, Description: svc.Description, State: state,
		Managed: managed, Healthy: healthy, Port: svc.Port, OpenURL: svc.OpenURL,
		Dependencies: append([]string(nil), svc.Dependencies...), StartedAt: startedAt,
		LastExitCode: lastExitCode, LastError: lastError,
		CanStart:   state == StateStopped || state == StateError,
		CanStop:    managed || healthy,
		CanRestart: managed || healthy,
	}
}

func (m *Manager) Start(ctx context.Context, id string) error {
	return m.start(ctx, id, make(map[string]bool))
}

func (m *Manager) start(ctx context.Context, id string, visiting map[string]bool) error {
	svc, ok := m.services[id]
	if !ok {
		return fmt.Errorf("unknown service %q", id)
	}
	if visiting[id] {
		return fmt.Errorf("dependency cycle at %q", id)
	}
	visiting[id] = true
	defer delete(visiting, id)
	for _, dependency := range svc.Dependencies {
		status := m.Status(ctx, dependency)
		if status.State != StateRunning {
			if err := m.start(ctx, dependency, visiting); err != nil {
				return fmt.Errorf("start dependency %s: %w", dependency, err)
			}
		}
	}
	if status := m.Status(ctx, id); status.State == StateRunning || status.State == StateStarting {
		return nil
	}
	if _, err := os.Stat(svc.Command); err != nil {
		return fmt.Errorf("command unavailable: %w", err)
	}
	if info, err := os.Stat(svc.WorkingDirectory); err != nil || !info.IsDir() {
		return fmt.Errorf("working directory unavailable")
	}
	rt := m.runtimes[id]
	rt.mu.Lock()
	if rt.cmd != nil && rt.cmd.ProcessState == nil {
		rt.mu.Unlock()
		return nil
	}
	cmd := exec.Command(svc.Command, svc.Args...)
	cmd.Dir = svc.WorkingDirectory
	cmd.Env = os.Environ()
	for key, value := range svc.Environment {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	configureProcess(cmd)
	cmd.Stdout = rt.logs
	cmd.Stderr = rt.logs
	rt.cmd = cmd
	rt.starting = true
	rt.stopping = false
	rt.startedAt = time.Now().UTC()
	rt.lastError = ""
	rt.lastExitCode = nil
	if err := cmd.Start(); err != nil {
		rt.cmd = nil
		rt.starting = false
		rt.lastError = err.Error()
		rt.mu.Unlock()
		m.audit(ctx, id, "start", "error", "Failed to start "+svc.Name+": "+err.Error())
		return err
	}
	rt.mu.Unlock()
	m.audit(ctx, id, "start", "info", "Started "+svc.Name)
	go m.wait(id, cmd)
	go m.clearStarting(id, svc.HealthURL)
	return nil
}

func (m *Manager) clearStarting(id, healthURL string) {
	m.clearStartingAfter(id, healthURL, 90*time.Second, time.Second)
}

func (m *Manager) clearStartingAfter(id, healthURL string, timeout, interval time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if healthURL == "" || m.healthy(context.Background(), healthURL) {
			rt := m.runtimes[id]
			rt.mu.Lock()
			rt.starting = false
			rt.lastError = ""
			rt.mu.Unlock()
			return
		}
		time.Sleep(interval)
	}
	rt := m.runtimes[id]
	rt.mu.Lock()
	if rt.starting {
		rt.starting = false
		rt.lastError = "service did not become healthy within " + timeout.Round(time.Second).String()
	}
	rt.mu.Unlock()
	m.audit(context.Background(), id, "health", "error", m.services[id].Name+" did not become healthy")
}

func (m *Manager) wait(id string, cmd *exec.Cmd) {
	err := cmd.Wait()
	rt := m.runtimes[id]
	rt.mu.Lock()
	if rt.cmd != cmd {
		rt.mu.Unlock()
		return
	}
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	stopping := rt.stopping
	rt.lastExitCode = &exitCode
	rt.cmd = nil
	rt.starting = false
	rt.stopping = false
	if err != nil && exitCode != -1 && !stopping {
		rt.lastError = err.Error()
	} else if stopping {
		rt.lastError = ""
	}
	rt.mu.Unlock()
	level := "info"
	if err != nil && exitCode != -1 && !stopping {
		level = "error"
	}
	m.audit(context.Background(), id, "exit", level, fmt.Sprintf("%s exited with code %d", m.services[id].Name, exitCode))
}

func (m *Manager) Stop(ctx context.Context, id string) error {
	svc, ok := m.services[id]
	if !ok {
		return fmt.Errorf("unknown service %q", id)
	}
	for dependentID, dependent := range m.services {
		if contains(dependent.Dependencies, id) && m.Status(ctx, dependentID).Managed {
			if err := m.Stop(ctx, dependentID); err != nil {
				return fmt.Errorf("stop dependent %s: %w", dependentID, err)
			}
		}
	}
	rt := m.runtimes[id]
	rt.mu.Lock()
	cmd := rt.cmd
	if cmd != nil && cmd.ProcessState == nil {
		rt.stopping = true
	}
	rt.mu.Unlock()
	if cmd == nil || cmd.ProcessState != nil {
		if m.Status(ctx, id).Healthy {
			pids, err := knownProcessIDs(svc)
			if err != nil {
				return fmt.Errorf("identify external service: %w", err)
			}
			var combined error
			for _, pid := range pids {
				combined = errors.Join(combined, terminatePIDTree(pid, time.Duration(svc.StopTimeoutSeconds)*time.Second))
			}
			if combined != nil {
				return fmt.Errorf("stop external process tree: %w", combined)
			}
			m.audit(ctx, id, "stop", "info", "Stopped externally detected "+svc.Name)
			return nil
		}
		return nil
	}
	timeout := time.Duration(m.services[id].StopTimeoutSeconds) * time.Second
	if err := terminateTree(cmd, timeout); err != nil {
		return fmt.Errorf("stop process tree: %w", err)
	}
	m.audit(ctx, id, "stop", "info", "Stopped "+m.services[id].Name)
	return nil
}

func (m *Manager) Restart(ctx context.Context, id string) error {
	if err := m.Stop(ctx, id); err != nil {
		return err
	}
	time.Sleep(350 * time.Millisecond)
	return m.Start(ctx, id)
}

func (m *Manager) StartGroup(ctx context.Context, id string) error {
	group, ok := m.groups[id]
	if !ok {
		return fmt.Errorf("unknown group %q", id)
	}
	for _, serviceID := range group.Services {
		if err := m.Start(ctx, serviceID); err != nil {
			return fmt.Errorf("start %s: %w", serviceID, err)
		}
	}
	m.audit(ctx, "", "group-start", "info", "Started "+group.Name)
	return nil
}

func (m *Manager) StopAll(ctx context.Context) error {
	var combined error
	for id := range m.services {
		if m.Status(ctx, id).CanStop {
			combined = errors.Join(combined, m.Stop(ctx, id))
		}
	}
	return combined
}

func (m *Manager) Logs(id string) (string, error) {
	rt, ok := m.runtimes[id]
	if !ok {
		return "", fmt.Errorf("unknown service %q", id)
	}
	return rt.logs.String(), nil
}

func (m *Manager) healthy(ctx context.Context, url string) bool {
	if url == "" {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	response, err := m.client.Do(req)
	if err != nil {
		return false
	}
	response.Body.Close()
	return response.StatusCode >= 200 && response.StatusCode < 500
}

func (m *Manager) audit(ctx context.Context, serviceID, action, level, message string) {
	if m.store != nil {
		_ = m.store.Add(ctx, serviceID, action, level, message)
	}
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func itoa(value int) string { return strconv.Itoa(value) }
