package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExpandsAndValidatesServices(t *testing.T) {
	t.Setenv("WORKBENCH_TEST_ROOT", t.TempDir())
	path := filepath.Join(t.TempDir(), "services.json")
	data := `{
		"services": [{
			"id": "engine", "name": "Engine", "description": "test",
			"command": "${WORKBENCH_TEST_ROOT}/engine.exe",
			"working_directory": "${WORKBENCH_TEST_ROOT}",
			"stop_timeout_seconds": 0
		}],
		"groups": [{"id":"default", "name":"Default", "services":["engine"]}]
	}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Services[0].StopTimeoutSeconds; got != 8 {
		t.Fatalf("default timeout = %d", got)
	}
	if cfg.Services[0].Command == "${WORKBENCH_TEST_ROOT}/engine.exe" {
		t.Fatal("command was not expanded")
	}
}

func TestLoadRejectsUnknownDependency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "services.json")
	data := `{"services":[{"id":"one","name":"One","command":"one.exe","working_directory":".","dependencies":["missing"]}]}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected dependency validation error")
	}
}
