package monitor

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	ctx "github.com/avaleror/claudewrap/internal/context"
)

// SessionInfo is written by the SessionStart hook and read by the TUI.
type SessionInfo struct {
	SessionID      string    `json:"session_id"`
	TranscriptPath string    `json:"transcript_path"`
	StartedAt      time.Time `json:"started_at"`
}

func SessionInfoPath(sessionID string) string {
	return filepath.Join(os.Getenv("HOME"), ".claudewrap", "sessions", sessionID+".json")
}

func WriteSessionInfo(info SessionInfo) error {
	dir := filepath.Dir(SessionInfoPath(info.SessionID))
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return os.WriteFile(SessionInfoPath(info.SessionID), data, 0644)
}

func ReadSessionInfo(sessionID string) (*SessionInfo, error) {
	data, err := os.ReadFile(SessionInfoPath(sessionID))
	if err != nil {
		return nil, err
	}
	var info SessionInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// State holds the live token state for a session, updated as JSONL entries arrive.
type State struct {
	mu sync.RWMutex

	FirstEntryTime      time.Time
	RemainingPct        float64
	UsedTokens          int
	TotalTokens         int
	ContextUsedPct      float64
	LastUsage           *Usage
	CompactionCount     int
	FallbackDailyCost   float64
	FallbackDailyTokens int
	FallbackEngine      string
	GitBranch           string
}

func NewState() *State {
	return &State{RemainingPct: 100}
}

func (s *State) Update(e Entry) {
	if e.Type != "assistant" && e.Type != "message" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.FirstEntryTime.IsZero() {
		s.FirstEntryTime = e.Timestamp
	}

	if e.Message == nil || e.Message.Usage == nil {
		return
	}
	u := e.Message.Usage
	s.LastUsage = u
	if u.RemainingPercentage > 0 {
		s.RemainingPct = u.RemainingPercentage
	}
	s.UsedTokens = u.InputTokens + u.OutputTokens
	if u.ContextWindow != nil {
		s.ContextUsedPct = u.ContextWindow.UsedPercentage
		s.TotalTokens = u.ContextWindow.TotalTokens
	}
}

func (s *State) IncrCompaction() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CompactionCount++
}

func (s *State) Snapshot() StateSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return StateSnapshot{
		RemainingPct:        s.RemainingPct,
		UsedTokens:          s.UsedTokens,
		TotalTokens:         s.TotalTokens,
		ContextUsedPct:      s.ContextUsedPct,
		LastUsage:           s.LastUsage,
		CompactionCount:     s.CompactionCount,
		FallbackDailyCost:   s.FallbackDailyCost,
		FallbackDailyTokens: s.FallbackDailyTokens,
		FallbackEngine:      s.FallbackEngine,
		GitBranch:           s.GitBranch,
		EstimatedReset:      estimatedResetTime(s.FirstEntryTime),
		IsPeak:              ctx.IsPeakHour(time.Now()),
	}
}

type StateSnapshot struct {
	RemainingPct        float64
	UsedTokens          int
	TotalTokens         int
	ContextUsedPct      float64
	LastUsage           *Usage
	CompactionCount     int
	FallbackDailyCost   float64
	FallbackDailyTokens int
	FallbackEngine      string
	GitBranch           string
	EstimatedReset      time.Time
	IsPeak              bool
}

func (s StateSnapshot) ResetIn() string {
	if s.EstimatedReset.IsZero() {
		return "unknown"
	}
	remaining := time.Until(s.EstimatedReset)
	if remaining <= 0 {
		return "now"
	}
	h := int(remaining.Hours())
	m := int(remaining.Minutes()) % 60
	return fmt.Sprintf("~%dh %02dm (est.)", h, m)
}

func estimatedResetTime(first time.Time) time.Time {
	if first.IsZero() {
		return time.Time{}
	}
	if ctx.IsPeakHour(time.Now()) {
		return first.Add(time.Duration(float64(5*time.Hour.Nanoseconds()) / 1.75))
	}
	return first.Add(5 * time.Hour)
}

func ProgressBar(pct float64, width int) string {
	filled := int(math.Round(float64(width) * pct / 100))
	if filled > width {
		filled = width
	}
	bar := ""
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "="
		} else if i == filled {
			bar += "-"
		} else {
			bar += " "
		}
	}
	return "[" + bar + "]"
}
