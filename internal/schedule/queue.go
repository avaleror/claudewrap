package schedule

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type QueuedPrompt struct {
	Prompt    string    `json:"prompt"`
	Bypass    bool      `json:"bypass"`
	AddedAt   time.Time `json:"added_at"`
}

func queuePath() string {
	return filepath.Join(os.Getenv("HOME"), ".claudewrap", "queue.json")
}

func Load() ([]QueuedPrompt, error) {
	data, err := os.ReadFile(queuePath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var q []QueuedPrompt
	return q, json.Unmarshal(data, &q)
}

func Append(prompt string, bypass bool) error {
	existing, _ := Load()
	existing = append(existing, QueuedPrompt{
		Prompt:  prompt,
		Bypass:  bypass,
		AddedAt: time.Now(),
	})
	return save(existing)
}

func Clear() error {
	return os.Remove(queuePath())
}

func HasQueue() bool {
	_, err := os.Stat(queuePath())
	return err == nil
}

func save(q []QueuedPrompt) error {
	data, err := json.MarshalIndent(q, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(queuePath())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(queuePath(), data, 0644)
}
