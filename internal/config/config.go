package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type Config struct {
	Services []Service `json:"services"`
	Groups   []Group   `json:"groups"`
}

type Service struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	Description        string            `json:"description"`
	Command            string            `json:"command"`
	Args               []string          `json:"args"`
	WorkingDirectory   string            `json:"working_directory"`
	Environment        map[string]string `json:"environment"`
	HealthURL          string            `json:"health_url"`
	OpenURL            string            `json:"open_url"`
	Port               int               `json:"port"`
	Dependencies       []string          `json:"dependencies"`
	StopTimeoutSeconds int               `json:"stop_timeout_seconds"`
}

type Group struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Services    []string `json:"services"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	seen := make(map[string]struct{}, len(cfg.Services))
	for i := range cfg.Services {
		svc := &cfg.Services[i]
		svc.Command = filepath.Clean(os.ExpandEnv(svc.Command))
		svc.WorkingDirectory = filepath.Clean(os.ExpandEnv(svc.WorkingDirectory))
		for key, value := range svc.Environment {
			svc.Environment[key] = os.ExpandEnv(value)
		}
		if !idPattern.MatchString(svc.ID) {
			return Config{}, fmt.Errorf("invalid service id %q", svc.ID)
		}
		if svc.Name == "" || svc.Command == "" || svc.WorkingDirectory == "" {
			return Config{}, fmt.Errorf("service %q is missing a required field", svc.ID)
		}
		if _, ok := seen[svc.ID]; ok {
			return Config{}, fmt.Errorf("duplicate service id %q", svc.ID)
		}
		seen[svc.ID] = struct{}{}
		if svc.StopTimeoutSeconds <= 0 {
			svc.StopTimeoutSeconds = 8
		}
	}
	for _, svc := range cfg.Services {
		for _, dependency := range svc.Dependencies {
			if _, ok := seen[dependency]; !ok {
				return Config{}, fmt.Errorf("service %q has unknown dependency %q", svc.ID, dependency)
			}
		}
	}
	for _, group := range cfg.Groups {
		if !idPattern.MatchString(group.ID) || group.Name == "" {
			return Config{}, fmt.Errorf("invalid group %q", group.ID)
		}
		for _, id := range group.Services {
			if _, ok := seen[id]; !ok {
				return Config{}, fmt.Errorf("group %q has unknown service %q", group.ID, id)
			}
		}
	}
	return cfg, nil
}
