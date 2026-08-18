package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestEventRoundTrip(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.Add(context.Background(), "ollama", "start", "info", "Started Ollama"); err != nil {
		t.Fatal(err)
	}
	events, err := database.Recent(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ServiceID != "ollama" || events[0].Action != "start" {
		t.Fatalf("unexpected events: %#v", events)
	}
}
