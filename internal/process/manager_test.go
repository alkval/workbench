package process

import (
	"testing"
	"time"

	"github.com/alkval/workbench/internal/config"
)

func TestClearStartingTimesOutToError(t *testing.T) {
	manager := New(config.Config{Services: []config.Service{{
		ID: "test", Name: "Test service", HealthURL: "http://127.0.0.1:1/health",
	}}}, nil)
	runtime := manager.runtimes["test"]
	runtime.starting = true

	manager.clearStartingAfter("test", "http://127.0.0.1:1/health", 20*time.Millisecond, 2*time.Millisecond)

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.starting {
		t.Fatal("startup flag was not cleared after the health timeout")
	}
	if runtime.lastError == "" {
		t.Fatal("health timeout did not record an actionable error")
	}
}
