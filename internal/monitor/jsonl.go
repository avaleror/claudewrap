package monitor

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Entry is a single JSONL line from a Claude Code session transcript.
type Entry struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Message   *Message  `json:"message,omitempty"`
}

type Message struct {
	Role    string   `json:"role"`
	Content []Content `json:"content,omitempty"`
	Usage   *Usage   `json:"usage,omitempty"`
}

type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type Usage struct {
	InputTokens              int     `json:"input_tokens"`
	OutputTokens             int     `json:"output_tokens"`
	CacheCreationInputTokens int     `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int     `json:"cache_read_input_tokens"`
	RemainingPercentage      float64 `json:"remaining_percentage"`
	ContextWindow            *ContextWindow `json:"context_window,omitempty"`
	Breakdown                *Breakdown     `json:"breakdown,omitempty"`
}

type ContextWindow struct {
	UsedPercentage float64 `json:"used_percentage"`
	TotalTokens    int     `json:"total_tokens"`
	UsedTokens     int     `json:"used_tokens"`
}

type Breakdown struct {
	ClaudeMD         int `json:"claude_md"`
	SkillActivations int `json:"skill_activations"`
	MentionedFiles   int `json:"mentioned_files"`
	ToolCallIO       int `json:"tool_call_io"`
	ExtendedThinking int `json:"extended_thinking"`
	TeamOverhead     int `json:"team_overhead"`
	UserText         int `json:"user_text"`
	Conversation     int `json:"conversation"`
}

// Watcher watches a JSONL file and emits new entries.
type Watcher struct {
	path    string
	watcher *fsnotify.Watcher
	mu      sync.Mutex
	offset  int64
	OnEntry func(Entry)
}

func NewWatcher(path string, onEntry func(Entry)) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &Watcher{
		path:    path,
		watcher: fw,
		OnEntry: onEntry,
	}
	// Replay existing entries
	w.readFrom(0)
	if err := fw.Add(path); err != nil {
		fw.Close()
		return nil, err
	}
	go w.run()
	return w, nil
}

func (w *Watcher) Close() {
	w.watcher.Close()
}

func (w *Watcher) run() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) {
				w.mu.Lock()
				w.readFrom(w.offset)
				w.mu.Unlock()
			}
		case _, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func (w *Watcher) readFrom(offset int64) {
	f, err := os.Open(w.path)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		var entry Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		w.offset += int64(len(line)) + 1
		if w.OnEntry != nil {
			w.OnEntry(entry)
		}
	}
}
