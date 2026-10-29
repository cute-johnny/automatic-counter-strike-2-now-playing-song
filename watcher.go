package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	pollInterval  = 500 * time.Millisecond
	maxEventQueue = 200
)

var (
	// Matches CS2 chat lines across [ALL], [T], [CT], [TEAM], [SPEC]:
	// e.g. "03/22 02:18:48  [ALL] PlayerName [DEAD]: message"
	chatLineRe = regexp.MustCompile(`(?i)(\d{2}/\d{2}\s+\d{2}:\d{2}:\d{2})\s+\[(ALL|T|CT|TEAM|SPEC)\]\s+(.+?)\s*:\s*(.+)`)

	// Strips trailing status tags like [DEAD], [SPEC], [BOT] from player names
	playerTagRe = regexp.MustCompile(`(?i)\s*\[(?:DEAD|SPEC|BOT)\]\s*$`)

	chatTokens = []string{"[ALL]", "[T]", "[CT]", "[TEAM]", "[SPEC]"}
)

func shortID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b[:])
}

// Trigger represents a single user-defined chat trigger rule.
type Trigger struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Pattern     string `json:"pattern"`
	Regex       string `json:"regex"`
	Enabled     bool   `json:"enabled"`
	StripPrefix string `json:"strip_prefix"`

	compiledRe   *regexp.Regexp
	regexInvalid bool
}

func (t *Trigger) compile() {
	t.compiledRe = nil
	t.regexInvalid = false
	if t.Regex != "" {
		re, err := regexp.Compile("(?i)" + t.Regex)
		if err != nil {
			t.regexInvalid = true
		} else {
			t.compiledRe = re
		}
	}
}

// Matches returns true if this trigger fires on the given chat message.
func (t *Trigger) Matches(message, messageLower string) bool {
	if !t.Enabled {
		return false
	}
	if t.Regex != "" {
		if t.compiledRe == nil && !t.regexInvalid {
			t.compile()
		}
		if t.compiledRe == nil {
			return false
		}
		return t.compiledRe.MatchString(message)
	}
	if t.Pattern == "" {
		return false
	}
	return strings.Contains(messageLower, strings.ToLower(t.Pattern))
}

// ExtractQuery extracts the search query from the message, stripping StripPrefix if set.
func (t *Trigger) ExtractQuery(message, messageLower string) string {
	if t.StripPrefix == "" {
		return strings.TrimSpace(message)
	}
	prefixLower := strings.ToLower(t.StripPrefix)
	idx := strings.Index(messageLower, prefixLower)
	if idx == -1 {
		return strings.TrimSpace(message)
	}
	afterIdx := idx + len(t.StripPrefix)
	if afterIdx > len(message) {
		afterIdx = len(message)
	}
	return strings.TrimSpace(message[afterIdx:])
}

// ChatEvent is fired when a chat line matches a trigger.
type ChatEvent struct {
	ID           string  `json:"id"`
	Timestamp    string  `json:"timestamp"`
	Channel      string  `json:"channel"`
	Player       string  `json:"player"`
	Message      string  `json:"message"`
	RawLine      string  `json:"raw_line"`
	TriggerID    string  `json:"trigger_id"`
	TriggerLabel string  `json:"trigger_label"`
	StripPrefix  string  `json:"strip_prefix"`
	SearchQuery  string  `json:"search_query"`
	Status       string  `json:"status"`
	VideoID      string  `json:"video_id,omitempty"`
	ReceivedAt   float64 `json:"received_at"`
}

// CS2ChatWatcher monitors CS2's console.log for new chat lines and dispatches ChatEvents.
type CS2ChatWatcher struct {
	LogPath string

	mu            sync.Mutex
	position      int64
	triggers      []Trigger
	callbacks     []func(*ChatEvent)
	events        []ChatEvent
	playEvents    []ChatEvent // Events explicitly dispatched for immediate browser playback
	banWords      map[string]struct{}
	channelFilter string // "both", "team_only", "all_only"
	running       bool
	stopCh        chan struct{}
	doneCh        chan struct{}
}

func NewCS2ChatWatcher(logPath string) *CS2ChatWatcher {
	return &CS2ChatWatcher{
		LogPath:       logPath,
		triggers:      make([]Trigger, 0),
		events:        make([]ChatEvent, 0, maxEventQueue),
		playEvents:    make([]ChatEvent, 0, maxEventQueue),
		banWords:      make(map[string]struct{}),
		channelFilter: "both",
	}
}

func (w *CS2ChatWatcher) SetChannelFilter(filter string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	switch filter {
	case "team_only", "all_only", "both":
		w.channelFilter = filter
	default:
		w.channelFilter = "both"
	}
}

func (w *CS2ChatWatcher) GetChannelFilter() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.channelFilter
}

func (w *CS2ChatWatcher) Start() {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.stopCh = make(chan struct{})
	w.doneCh = make(chan struct{})
	w.seekToEndLocked()
	w.mu.Unlock()

	go w.watchLoop()
	logMsg("INFO", "CS2ChatWatcher started — watching: "+w.LogPath)
}

func (w *CS2ChatWatcher) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	close(w.stopCh)
	done := w.doneCh
	w.mu.Unlock()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
}

func (w *CS2ChatWatcher) IsRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

func (w *CS2ChatWatcher) LogExists() bool {
	info, err := os.Stat(w.LogPath)
	return err == nil && info.Mode().IsRegular()
}

func (w *CS2ChatWatcher) AddCallback(fn func(*ChatEvent)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.callbacks = append(w.callbacks, fn)
}

func (w *CS2ChatWatcher) AddTrigger(t Trigger) Trigger {
	t.compile()
	w.mu.Lock()
	replaced := false
	for i := range w.triggers {
		if w.triggers[i].ID == t.ID {
			w.triggers[i] = t
			replaced = true
			break
		}
	}
	if !replaced {
		w.triggers = append(w.triggers, t)
	}
	w.mu.Unlock()
	return t
}

func (w *CS2ChatWatcher) UpdateTrigger(id string, updates map[string]any) *Trigger {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i := range w.triggers {
		if w.triggers[i].ID == id {
			t := &w.triggers[i]
			if v, ok := updates["label"].(string); ok {
				t.Label = strings.TrimSpace(v)
			}
			if v, ok := updates["pattern"].(string); ok {
				t.Pattern = strings.TrimSpace(v)
			}
			if v, ok := updates["regex"].(string); ok {
				t.Regex = strings.TrimSpace(v)
			}
			if v, ok := updates["strip_prefix"].(string); ok {
				t.StripPrefix = strings.TrimSpace(v)
			}
			if v, ok := updates["enabled"].(bool); ok {
				t.Enabled = v
			}
			t.compile()
			copyT := *t
			return &copyT
		}
	}
	return nil
}

func (w *CS2ChatWatcher) RemoveTrigger(id string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	for i := range w.triggers {
		if w.triggers[i].ID == id {
			w.triggers = append(w.triggers[:i], w.triggers[i+1:]...)
			return true
		}
	}
	return false
}

func (w *CS2ChatWatcher) GetTriggers() []Trigger {
	w.mu.Lock()
	defer w.mu.Unlock()

	out := make([]Trigger, len(w.triggers))
	copy(out, w.triggers)
	return out
}

// GetRecentEvents returns recent chat trigger events for the web panel log.
func (w *CS2ChatWatcher) GetRecentEvents(limit int) []ChatEvent {
	w.mu.Lock()
	defer w.mu.Unlock()

	n := len(w.events)
	if limit <= 0 || limit > n {
		limit = n
	}
	out := make([]ChatEvent, 0, limit)
	for i := n - 1; i >= n-limit; i-- {
		out = append(out, w.events[i])
	}
	return out
}

// GetPlayEvents returns events that have been approved/dispatched for immediate playback by the Queue Engine.
func (w *CS2ChatWatcher) GetPlayEvents(limit int) []ChatEvent {
	w.mu.Lock()
	defer w.mu.Unlock()

	n := len(w.playEvents)
	if limit <= 0 || limit > n {
		limit = n
	}
	out := make([]ChatEvent, 0, limit)
	for i := n - 1; i >= n-limit; i-- {
		out = append(out, w.playEvents[i])
	}
	return out
}

// PushPlayEvent dispatches an event for immediate playback in the browser userscript and records it in the UI log.
func (w *CS2ChatWatcher) PushPlayEvent(evt ChatEvent) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.appendPlayEventLocked(evt)
	w.appendEventLocked(evt)
}

// PushPlayOnly dispatches an event to the browser userscript without duplicating an existing UI log entry.
func (w *CS2ChatWatcher) PushPlayOnly(evt ChatEvent) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.appendPlayEventLocked(evt)
}

func (w *CS2ChatWatcher) PushEvent(evt ChatEvent) {
	w.PushPlayEvent(evt)
}

func (w *CS2ChatWatcher) appendEventLocked(evt ChatEvent) {
	if len(w.events) >= maxEventQueue {
		copy(w.events, w.events[1:])
		w.events[len(w.events)-1] = evt
	} else {
		w.events = append(w.events, evt)
	}
}

func (w *CS2ChatWatcher) appendPlayEventLocked(evt ChatEvent) {
	if len(w.playEvents) >= maxEventQueue {
		copy(w.playEvents, w.playEvents[1:])
		w.playEvents[len(w.playEvents)-1] = evt
	} else {
		w.playEvents = append(w.playEvents, evt)
	}
}

func (w *CS2ChatWatcher) SetBanWords(words []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.banWords = make(map[string]struct{}, len(words))
	for _, word := range words {
		clean := strings.ToLower(strings.TrimSpace(word))
		if clean != "" {
			w.banWords[clean] = struct{}{}
		}
	}
}

func (w *CS2ChatWatcher) AddBanWord(word string) bool {
	clean := strings.ToLower(strings.TrimSpace(word))
	if clean == "" {
		return false
	}
	w.mu.Lock()
	_, exists := w.banWords[clean]
	w.banWords[clean] = struct{}{}
	w.mu.Unlock()
	return !exists
}

func (w *CS2ChatWatcher) RemoveBanWord(word string) bool {
	clean := strings.ToLower(strings.TrimSpace(word))
	w.mu.Lock()
	_, exists := w.banWords[clean]
	delete(w.banWords, clean)
	w.mu.Unlock()
	return exists
}

func (w *CS2ChatWatcher) GetBanWords() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]string, 0, len(w.banWords))
	for bw := range w.banWords {
		out = append(out, bw)
	}
	sort.Strings(out)
	return out
}

func (w *CS2ChatWatcher) seekToEndLocked() {
	info, err := os.Stat(w.LogPath)
	if err == nil && info.Mode().IsRegular() {
		w.position = info.Size()
	} else {
		w.position = 0
	}
}

func (w *CS2ChatWatcher) watchLoop() {
	defer close(w.doneCh)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		w.readNewLines()
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
		}
	}
}

func (w *CS2ChatWatcher) readNewLines() {
	info, err := os.Stat(w.LogPath)
	if err != nil || !info.Mode().IsRegular() {
		return
	}
	fileSize := info.Size()

	w.mu.Lock()
	pos := w.position
	if fileSize < pos {
		pos = 0
		w.position = 0
	}
	w.mu.Unlock()

	if fileSize == pos {
		return
	}

	f, err := os.Open(w.LogPath)
	if err != nil {
		return
	}
	defer f.Close()

	if _, err := f.Seek(pos, io.SeekStart); err != nil {
		return
	}
	raw, err := io.ReadAll(f)
	if err != nil || len(raw) == 0 {
		return
	}

	// Only consume complete lines ending in '\n' so we never split a line mid-flush
	lastNL := bytes.LastIndexByte(raw, '\n')
	if lastNL == -1 {
		return
	}

	w.mu.Lock()
	w.position = pos + int64(lastNL+1)
	w.mu.Unlock()

	raw = raw[:lastNL+1]
	content := strings.ReplaceAll(string(raw), "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	for _, line := range strings.Split(content, "\n") {
		if line != "" {
			w.processLine(line)
		}
	}
}

func hasChatToken(line string) bool {
	for _, tok := range chatTokens {
		if strings.Contains(line, tok) {
			return true
		}
	}
	return false
}

func (w *CS2ChatWatcher) processLine(line string) {
	if !hasChatToken(line) {
		return
	}

	m := chatLineRe.FindStringSubmatch(line)
	if m == nil {
		return
	}

	timestamp := m[1]
	channel := strings.ToUpper(m[2])
	player := strings.TrimSpace(playerTagRe.ReplaceAllString(m[3], ""))
	message := strings.TrimSpace(m[4])
	messageLower := strings.ToLower(message)

	w.mu.Lock()
	chFilter := w.channelFilter
	for bw := range w.banWords {
		if strings.Contains(messageLower, bw) {
			w.mu.Unlock()
			return
		}
	}
	triggersSnapshot := make([]Trigger, len(w.triggers))
	copy(triggersSnapshot, w.triggers)
	w.mu.Unlock()

	// Enforce channel filter ("both", "team_only", "all_only")
	if chFilter == "all_only" && channel != "ALL" {
		return
	}
	if chFilter == "team_only" && channel == "ALL" {
		return
	}

	nowSec := float64(time.Now().UnixNano()) / 1e9
	for i := range triggersSnapshot {
		trig := &triggersSnapshot[i]
		if trig.Matches(message, messageLower) {
			evt := ChatEvent{
				ID:           shortID(),
				Timestamp:    timestamp,
				Channel:      channel,
				Player:       player,
				Message:      message,
				RawLine:      line,
				TriggerID:    trig.ID,
				TriggerLabel: trig.Label,
				StripPrefix:  trig.StripPrefix,
				SearchQuery:  trig.ExtractQuery(message, messageLower),
				ReceivedAt:   nowSec,
			}
			w.dispatch(evt)
		}
	}
}

func (w *CS2ChatWatcher) dispatch(evt ChatEvent) {
	w.mu.Lock()
	cbs := make([]func(*ChatEvent), len(w.callbacks))
	copy(cbs, w.callbacks)
	w.mu.Unlock()

	logMsg("MUSIC", "MATCH ["+evt.TriggerLabel+"] "+evt.Player+": "+evt.Message)

	// Allow callbacks (such as the Queue Engine) to inspect, route, and set evt.Status
	for _, fn := range cbs {
		func() {
			defer func() { _ = recover() }()
			fn(&evt)
		}()
	}

	w.mu.Lock()
	w.appendEventLocked(evt)
	w.mu.Unlock()
}

// FindConsoleLog derives the console.log path from either the nowplaying.cfg file path or cfg folder path.
func FindConsoleLog(cs2CfgPath string) string {
	p := cs2CfgPath
	info, err := os.Stat(p)
	if (err == nil && !info.IsDir()) || strings.HasSuffix(strings.ToLower(p), ".cfg") {
		p = filepath.Dir(p)
	}
	csgoDir := filepath.Dir(p)
	return filepath.Join(csgoDir, "console.log")
}
