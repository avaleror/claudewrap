package schedule

import (
	"encoding/json"
	"fmt"
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

// ContextSnapshot captures token state at the moment of rate limiting.
type ContextSnapshot struct {
	Timestamp       time.Time `json:"timestamp"`
	RemainingPct    float64   `json:"remaining_pct"`
	UsedTokens      int       `json:"used_tokens"`
	TotalTokens     int       `json:"total_tokens"`
	EstimatedReset  time.Time `json:"estimated_reset"`
	CompactionCount int       `json:"compaction_count"`
}

func SaveContextSnapshot(snap ContextSnapshot) error {
	snap.Timestamp = time.Now()
	name := fmt.Sprintf("context_%s.json", snap.Timestamp.Format("20060102-150405"))
	path := filepath.Join(os.Getenv("HOME"), ".claudewrap", name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
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
