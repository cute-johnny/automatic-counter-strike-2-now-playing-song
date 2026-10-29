package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed templates/index.html static/*
var embeddedAssets embed.FS

// ─────────────────────────────────────────────
//  SOURCE ENGINE COLOR CODES
// ─────────────────────────────────────────────

var colorCodes = map[string]string{
	"none":       "",
	"default":    "\x01",
	"team":       "\x04",
	"lightgreen": "\x05",
	"green":      "\x06",
	"olive":      "\x08",
	"gold":       "\x09",
	"silver":     "\x0A",
	"red":        "\x0B",
	"pink":       "\x0C",
	"lightred":   "\x0D",
	"purple":     "\x0E",
	"lightblue":  "\x10",
}

// ─────────────────────────────────────────────
//  UNICODE FONT CONVERTER
// ─────────────────────────────────────────────

type fontMap map[rune]string

var unicodeFonts map[string]fontMap

func makeFontTable(upperStart, lowerStart, digitStart rune, upperEx, lowerEx map[rune]rune) fontMap {
	m := make(fontMap, 62)
	for i := rune(0); i < 26; i++ {
		u := 'A' + i
		if ex, ok := upperEx[u]; ok {
			m[u] = string(ex)
		} else {
			m[u] = string(upperStart + i)
		}
		l := 'a' + i
		if ex, ok := lowerEx[l]; ok {
			m[l] = string(ex)
		} else {
			m[l] = string(lowerStart + i)
		}
	}
	if digitStart != 0 {
		for i := rune(0); i < 10; i++ {
			m['0'+i] = string(digitStart + i)
		}
	}
	return m
}

func init() {
	italicUpperEx := map[rune]rune{'H': 'ℋ'}
	italicLowerEx := map[rune]rune{'h': 'ℎ', 'e': 'ℯ', 'g': 'ℊ', 'o': 'ℴ'}
	scriptUpperEx := map[rune]rune{'B': 'ℬ', 'E': 'ℰ', 'F': 'ℱ', 'H': 'ℋ', 'I': 'ℐ', 'L': 'ℒ', 'M': 'ℳ', 'R': 'ℛ'}
	scriptLowerEx := map[rune]rune{'e': 'ℯ', 'g': 'ℊ', 'o': 'ℴ'}
	frakturUpperEx := map[rune]rune{'C': 'ℭ', 'H': 'ℌ', 'I': 'ℑ', 'R': 'ℜ', 'Z': 'ℨ'}
	dstruckUpperEx := map[rune]rune{'C': 'ℂ', 'H': 'ℍ', 'N': 'ℕ', 'P': 'ℙ', 'Q': 'ℚ', 'R': 'ℝ', 'Z': 'ℤ'}

	smallCapsRunes := []rune("ᴀʙᴄᴅᴇꜰɢʜɪᴊᴋʟᴍɴᴏᴘǫʀsᴛᴜᴠᴡxʏᴢ")
	smallCapsMap := make(fontMap, 26)
	for i := 0; i < 26; i++ {
		smallCapsMap[rune('a'+i)] = string(smallCapsRunes[i])
	}

	circledMap := make(fontMap, 62)
	for i := rune(0); i < 26; i++ {
		circledMap['A'+i] = string(rune(0x24B6) + i)
		circledMap['a'+i] = string(rune(0x24D0) + i)
	}
	for i := rune(1); i <= 9; i++ {
		circledMap['0'+i] = string(rune(0x2460) + i - 1)
	}
	circledMap['0'] = "⓪"

	strikeMap := make(fontMap, 62)
	for _, r := range "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789" {
		strikeMap[r] = string(r) + "\u0336"
	}

	unicodeFonts = map[string]fontMap{
		"normal":        nil,
		"bold":          makeFontTable(0x1D400, 0x1D41A, 0x1D7CE, nil, nil),
		"italic":        makeFontTable(0x1D434, 0x1D44E, 0, italicUpperEx, italicLowerEx),
		"bold_italic":   makeFontTable(0x1D468, 0x1D482, 0x1D7CE, nil, nil),
		"script":        makeFontTable(0x1D49C, 0x1D4B6, 0, scriptUpperEx, scriptLowerEx),
		"bold_script":   makeFontTable(0x1D4D0, 0x1D4EA, 0, nil, nil),
		"fraktur":       makeFontTable(0x1D504, 0x1D51E, 0, frakturUpperEx, nil),
		"double":        makeFontTable(0x1D538, 0x1D552, 0x1D7D8, dstruckUpperEx, nil),
		"mono":          makeFontTable(0x1D670, 0x1D68A, 0x1D7F6, nil, nil),
		"wide":          makeFontTable(0xFF21, 0xFF41, 0xFF10, nil, nil),
		"small_caps":    smallCapsMap,
		"circled":       circledMap,
		"strikethrough": strikeMap,
	}
}

func applyFont(text, font string) string {
	table, ok := unicodeFonts[font]
	if !ok || table == nil {
		return text
	}
	var sb strings.Builder
	sb.Grow(len(text) * 2)
	for _, r := range text {
		if repl, found := table[r]; found {
			sb.WriteString(repl)
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// ─────────────────────────────────────────────
//  LIVE CONFIG (MESSAGE BUILDER)
// ─────────────────────────────────────────────

type Block struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Value string `json:"value"`
	Color string `json:"color"`
	Font  string `json:"font"`
}

var defaultBlocks = []Block{
	{ID: "b1", Type: "text", Value: "Now Playing:", Color: "gold", Font: "normal"},
	{ID: "b2", Type: "artist", Value: "", Color: "silver", Font: "normal"},
	{ID: "b3", Type: "title", Value: "", Color: "lightgreen", Font: "normal"},
}

func isValidBlockType(t string) bool {
	switch t {
	case "text", "artist", "title", "time", "duration", "requester":
		return true
	default:
		return false
	}
}

func sanitizeBlocks(raw []Block) []Block {
	valid := make([]Block, 0, len(raw))
	for _, b := range raw {
		if !isValidBlockType(b.Type) {
			continue
		}
		id := strings.TrimSpace(b.ID)
		if id == "" {
			id = shortID()
		}
		color := b.Color
		if _, ok := colorCodes[color]; !ok {
			color = "none"
		}
		font := b.Font
		if _, ok := unicodeFonts[font]; !ok {
			font = "normal"
		}
		valid = append(valid, Block{
			ID:    id,
			Type:  b.Type,
			Value: b.Value,
			Color: color,
			Font:  font,
		})
	}
	return valid
}

type ConfigData struct {
	Blocks         []Block `json:"blocks"`
	UseColors      bool    `json:"use_colors"`
	ChatCommand    string  `json:"chat_command"`    // "say" or "say_team"
	BlockSeparator string  `json:"block_separator"` // default " "
}

type LiveConfig struct {
	mu             sync.Mutex
	blocks         []Block
	useColors      bool
	chatCommand    string
	blockSeparator string
	hasTimeBlocks  bool
}

func NewLiveConfig() *LiveConfig {
	blocks := make([]Block, len(defaultBlocks))
	copy(blocks, defaultBlocks)
	return &LiveConfig{
		blocks:         blocks,
		useColors:      false,
		chatCommand:    "say",
		blockSeparator: " ",
		hasTimeBlocks:  false,
	}
}

func (lc *LiveConfig) ToDict() ConfigData {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	cp := make([]Block, len(lc.blocks))
	copy(cp, lc.blocks)
	return ConfigData{
		Blocks:         cp,
		UseColors:      lc.useColors,
		ChatCommand:    lc.chatCommand,
		BlockSeparator: lc.blockSeparator,
	}
}

func (lc *LiveConfig) Update(blocks *[]Block, useColors *bool, chatCommand *string, blockSeparator *string) {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	if blocks != nil {
		valid := sanitizeBlocks(*blocks)
		lc.blocks = valid
		hasTime := false
		for _, b := range valid {
			if b.Type == "time" || b.Type == "duration" {
				hasTime = true
				break
			}
		}
		lc.hasTimeBlocks = hasTime
	}
	if useColors != nil {
		lc.useColors = *useColors
	}
	if chatCommand != nil {
		if *chatCommand == "say_team" {
			lc.chatCommand = "say_team"
		} else {
			lc.chatCommand = "say"
		}
	}
	if blockSeparator != nil {
		lc.blockSeparator = *blockSeparator
	}
}

func (lc *LiveConfig) HasTimeBlocks() bool {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	return lc.hasTimeBlocks
}

func (lc *LiveConfig) Format(title, artist, currentTime, duration, requester string) string {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	parts := make([]string, 0, len(lc.blocks))
	for _, block := range lc.blocks {
		bcolor := ""
		if lc.useColors {
			bcolor = colorCodes[block.Color]
		}
		bfont := block.Font
		if bfont == "" {
			bfont = "normal"
		}

		switch block.Type {
		case "text":
			val := strings.TrimSpace(block.Value)
			if val != "" {
				parts = append(parts, bcolor+applyFont(val, bfont))
			}
		case "artist":
			if artist != "" {
				parts = append(parts, bcolor+applyFont(artist, bfont))
			}
		case "title":
			if title != "" {
				parts = append(parts, bcolor+applyFont(title, bfont))
			}
		case "time":
			if currentTime != "" {
				parts = append(parts, bcolor+applyFont(currentTime, bfont))
			}
		case "duration":
			if duration != "" {
				parts = append(parts, bcolor+applyFont(duration, bfont))
			}
		case "requester":
			if requester != "" {
				parts = append(parts, bcolor+applyFont(requester, bfont))
			}
		}
	}
	sep := lc.blockSeparator
	if sep == "" {
		sep = " "
	}
	return strings.Join(parts, sep)
}

func (lc *LiveConfig) GetCommand(title, artist, currentTime, duration, requester string) string {
	message := lc.Format(title, artist, currentTime, duration, requester)
	lc.mu.Lock()
	useColors := lc.useColors
	chatCmd := lc.chatCommand
	lc.mu.Unlock()

	if chatCmd != "say_team" {
		chatCmd = "say"
	}

	safe := strings.NewReplacer(
		`"`, `'`,
		`;`, `,`,
		"\n", " ",
		"\r", " ",
		`\`, "",
	).Replace(message)

	cmd := chatCmd
	if useColors {
		cmd = "playerchatwheel CW.Agree"
	}
	return fmt.Sprintf(`%s "%s"`, cmd, safe)
}

// ─────────────────────────────────────────────
//  SONG HISTORY & PLAYBACK STATE
// ─────────────────────────────────────────────

type SongHistory struct {
	mu                 sync.Mutex
	lastTitle          string
	lastArtist         string
	lastTime           string
	lastDuration       string
	lastRequester      string
	lastTimeTickAt     time.Time
	lastDispatchedAt   time.Time
	playbackEnded      bool
	autoSkipTriggered  bool
}

func (sh *SongHistory) IsNew(title, artist string) bool {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if sh.lastTitle == title && sh.lastArtist == artist {
		return false
	}
	sh.lastTitle = title
	sh.lastArtist = artist
	sh.playbackEnded = false
	sh.autoSkipTriggered = false
	sh.lastTimeTickAt = time.Now()
	return true
}

func (sh *SongHistory) SetSong(title, artist, currentTime, duration, requester string) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.lastTitle = title
	sh.lastArtist = artist
	sh.lastTime = currentTime
	sh.lastDuration = duration
	if requester != "" {
		sh.lastRequester = requester
	}
	sh.playbackEnded = false
	sh.autoSkipTriggered = false
	sh.lastDispatchedAt = time.Now()
	sh.lastTimeTickAt = time.Now()
}

func (sh *SongHistory) UpdateTime(currentTime, duration string) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if sh.lastTime != currentTime {
		sh.lastTimeTickAt = time.Now()
		sh.playbackEnded = false
	}
	sh.lastTime = currentTime
	if duration != "" {
		sh.lastDuration = duration
	}
}

func (sh *SongHistory) MarkEnded() {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	sh.playbackEnded = true
}

func (sh *SongHistory) TryMarkAutoSkip() bool {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if sh.autoSkipTriggered {
		return false
	}
	sh.autoSkipTriggered = true
	return true
}

// IsCurrentlyPlaying returns true if a track is actively playing or was just dispatched.
func (sh *SongHistory) IsCurrentlyPlaying() bool {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if sh.lastTitle == "" || sh.playbackEnded {
		return false
	}
	now := time.Now()
	// Grace window right after dispatching a new song (while YouTube/SoundCloud loads)
	if !sh.lastDispatchedAt.IsZero() && now.Sub(sh.lastDispatchedAt) < 15*time.Second {
		return true
	}
	// Active time updates within the last 6 seconds
	if !sh.lastTimeTickAt.IsZero() && now.Sub(sh.lastTimeTickAt) < 6*time.Second {
		return true
	}
	return false
}

func (sh *SongHistory) Snapshot() (title, artist, curTime, duration, requester string) {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	return sh.lastTitle, sh.lastArtist, sh.lastTime, sh.lastDuration, sh.lastRequester
}

// ─────────────────────────────────────────────
//  REWORKED SONG QUEUE & SETTINGS
// ─────────────────────────────────────────────

type QueueSettings struct {
	PlaybackMode         string `json:"playback_mode"`           // "auto_queue", "instant", "manual"
	CooldownSec          int    `json:"cooldown_sec"`            // Global cooldown between chat triggers (0 = off)
	PerPlayerCooldownSec int    `json:"per_player_cooldown_sec"` // Per-player cooldown (0 = off)
	MaxQueueSize         int    `json:"max_queue_size"`          // 1..100
	MaxPerPlayer         int    `json:"max_per_player"`          // 0 = unlimited
	PreventDuplicates    bool   `json:"prevent_duplicates"`      // Disallow duplicate songs
	MaxDurationMin       int    `json:"max_duration_min"`        // 0 = unlimited, auto-skip if exceeded
	ChatChannels         string `json:"chat_channels"`           // "both", "team_only", "all_only"
}

func defaultQueueSettings() QueueSettings {
	return QueueSettings{
		PlaybackMode:         "auto_queue",
		CooldownSec:          5,
		PerPlayerCooldownSec: 15,
		MaxQueueSize:         20,
		MaxPerPlayer:         3,
		PreventDuplicates:    true,
		MaxDurationMin:       10,
		ChatChannels:         "both",
	}
}

func sanitizeQueueSettings(s QueueSettings) QueueSettings {
	switch s.PlaybackMode {
	case "auto_queue", "instant", "manual":
	default:
		s.PlaybackMode = "auto_queue"
	}
	if s.CooldownSec < 0 {
		s.CooldownSec = 0
	} else if s.CooldownSec > 600 {
		s.CooldownSec = 600
	}
	if s.PerPlayerCooldownSec < 0 {
		s.PerPlayerCooldownSec = 0
	} else if s.PerPlayerCooldownSec > 1800 {
		s.PerPlayerCooldownSec = 1800
	}
	if s.MaxQueueSize < 1 {
		s.MaxQueueSize = 1
	} else if s.MaxQueueSize > 100 {
		s.MaxQueueSize = 100
	}
	if s.MaxPerPlayer < 0 {
		s.MaxPerPlayer = 0
	} else if s.MaxPerPlayer > 50 {
		s.MaxPerPlayer = 50
	}
	if s.MaxDurationMin < 0 {
		s.MaxDurationMin = 0
	} else if s.MaxDurationMin > 180 {
		s.MaxDurationMin = 180
	}
	switch s.ChatChannels {
	case "both", "team_only", "all_only":
	default:
		s.ChatChannels = "both"
	}
	return s
}

type QueuedSong struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	Query     string `json:"query"`
	VideoID   string `json:"video_id,omitempty"`
	Duration  string `json:"duration,omitempty"`
	Requester string `json:"requester"`
	Source    string `json:"source"`
	AddedAt   string `json:"added_at"`
	Position  int    `json:"position"`
}

type SongQueue struct {
	mu               sync.Mutex
	queue            []QueuedSong
	processing       bool
	settings         QueueSettings
	lastTriggerAt    time.Time
	playerLastTrigAt map[string]time.Time
}

func NewSongQueue() *SongQueue {
	return &SongQueue{
		queue:            make([]QueuedSong, 0),
		settings:         defaultQueueSettings(),
		playerLastTrigAt: make(map[string]time.Time),
	}
}

func (sq *SongQueue) GetSettings() QueueSettings {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	return sq.settings
}

func (sq *SongQueue) UpdateSettings(s QueueSettings) QueueSettings {
	clean := sanitizeQueueSettings(s)
	sq.mu.Lock()
	sq.settings = clean
	sq.mu.Unlock()
	if watcher != nil {
		watcher.SetChannelFilter(clean.ChatChannels)
	}
	return clean
}

// CheckCooldowns verifies global and per-player cooldowns for chat triggers.
func (sq *SongQueue) CheckCooldowns(player string) (bool, string) {
	sq.mu.Lock()
	defer sq.mu.Unlock()

	now := time.Now()
	if sq.settings.CooldownSec > 0 && !sq.lastTriggerAt.IsZero() {
		rem := time.Duration(sq.settings.CooldownSec)*time.Second - now.Sub(sq.lastTriggerAt)
		if rem > 0 {
			return false, fmt.Sprintf("global cooldown (%ds left)", int(rem.Seconds())+1)
		}
	}
	if player != "" && sq.settings.PerPlayerCooldownSec > 0 {
		pKey := strings.ToLower(player)
		if last, ok := sq.playerLastTrigAt[pKey]; ok {
			rem := time.Duration(sq.settings.PerPlayerCooldownSec)*time.Second - now.Sub(last)
			if rem > 0 {
				return false, fmt.Sprintf("player cooldown (%ds left)", int(rem.Seconds())+1)
			}
		}
	}
	return true, ""
}

func (sq *SongQueue) RecordTriggerTime(player string) {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	now := time.Now()
	sq.lastTriggerAt = now
	if player != "" {
		sq.playerLastTrigAt[strings.ToLower(player)] = now
	}
}

func normalizeSongKey(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// IsDuplicate checks if the same song/query/videoId is already queued or currently playing.
func (sq *SongQueue) IsDuplicate(query, title, artist, videoID string) bool {
	sq.mu.Lock()
	prevent := sq.settings.PreventDuplicates
	qCopy := make([]QueuedSong, len(sq.queue))
	copy(qCopy, sq.queue)
	sq.mu.Unlock()

	if !prevent {
		return false
	}

	qKey := normalizeSongKey(query)
	fullTitle := title
	if artist != "" {
		fullTitle = artist + " - " + title
	}
	tKey := normalizeSongKey(fullTitle)

	// Check against currently playing track
	if songs.IsCurrentlyPlaying() {
		curTitle, curArtist, _, _, _ := songs.Snapshot()
		curFull := curTitle
		if curArtist != "" {
			curFull = curArtist + " - " + curTitle
		}
		curKey := normalizeSongKey(curFull)
		if (tKey != "" && curKey == tKey) || (qKey != "" && curKey == qKey) {
			return true
		}
	}

	// Check against queued items
	for _, item := range qCopy {
		if videoID != "" && item.VideoID != "" && item.VideoID == videoID {
			return true
		}
		if qKey != "" && normalizeSongKey(item.Query) == qKey {
			return true
		}
		itemFull := item.Title
		if item.Artist != "" {
			itemFull = item.Artist + " - " + item.Title
		}
		if tKey != "" && normalizeSongKey(itemFull) == tKey {
			return true
		}
	}
	return false
}

// AddWithRules validates queue size and per-player limits and appends the song.
func (sq *SongQueue) AddWithRules(song QueuedSong, enforcePlayerLimit bool) (int, string) {
	sq.mu.Lock()
	defer sq.mu.Unlock()

	if len(sq.queue) >= sq.settings.MaxQueueSize {
		return 0, fmt.Sprintf("queue is full (max %d)", sq.settings.MaxQueueSize)
	}

	if enforcePlayerLimit && sq.settings.MaxPerPlayer > 0 && song.Requester != "" {
		reqLower := strings.ToLower(song.Requester)
		count := 0
		for _, existing := range sq.queue {
			if strings.ToLower(existing.Requester) == reqLower {
				count++
			}
		}
		if count >= sq.settings.MaxPerPlayer {
			return 0, fmt.Sprintf("max %d songs per player in queue", sq.settings.MaxPerPlayer)
		}
	}

	if song.ID == "" {
		song.ID = shortID()
	}
	if song.AddedAt == "" {
		song.AddedAt = time.Now().Format("2006-01-02 15:04:05")
	}
	song.Position = len(sq.queue) + 1
	sq.queue = append(sq.queue, song)
	return song.Position, ""
}

func (sq *SongQueue) Add(title, artist, source string) bool {
	q := title
	if artist != "" {
		q = artist + " - " + title
	}
	requester := source
	if strings.HasPrefix(source, "chat:") {
		requester = strings.TrimPrefix(source, "chat:")
	}
	pos, errReason := sq.AddWithRules(QueuedSong{
		Title:     title,
		Artist:    artist,
		Query:     q,
		Requester: requester,
		Source:    source,
	}, false)
	return pos > 0 && errReason == ""
}

func (sq *SongQueue) Remove(songID string) bool {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	for i, s := range sq.queue {
		if s.ID == songID {
			sq.queue = append(sq.queue[:i], sq.queue[i+1:]...)
			for j := i; j < len(sq.queue); j++ {
				sq.queue[j].Position = j + 1
			}
			return true
		}
	}
	return false
}

func (sq *SongQueue) PopByID(songID string) *QueuedSong {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	for i, s := range sq.queue {
		if s.ID == songID {
			picked := s
			sq.queue = append(sq.queue[:i], sq.queue[i+1:]...)
			for j := i; j < len(sq.queue); j++ {
				sq.queue[j].Position = j + 1
			}
			return &picked
		}
	}
	return nil
}

func (sq *SongQueue) GetQueue() []QueuedSong {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	cp := make([]QueuedSong, len(sq.queue))
	copy(cp, sq.queue)
	return cp
}

func (sq *SongQueue) Clear() {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	sq.queue = sq.queue[:0]
}

func (sq *SongQueue) GetNext() *QueuedSong {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	if len(sq.queue) == 0 {
		return nil
	}
	next := sq.queue[0]
	sq.queue = sq.queue[1:]
	for i := range sq.queue {
		sq.queue[i].Position = i + 1
	}
	return &next
}

func (sq *SongQueue) Move(songID string, newPos int) bool {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	if newPos < 1 || newPos > len(sq.queue) {
		return false
	}
	for i, s := range sq.queue {
		if s.ID == songID {
			sq.queue = append(sq.queue[:i], sq.queue[i+1:]...)
			idx := newPos - 1
			sq.queue = append(sq.queue[:idx], append([]QueuedSong{s}, sq.queue[idx:]...)...)
			for j := range sq.queue {
				sq.queue[j].Position = j + 1
			}
			return true
		}
	}
	return false
}

func (sq *SongQueue) Size() int {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	return len(sq.queue)
}

func (sq *SongQueue) IsEmpty() bool {
	return sq.Size() == 0
}

func (sq *SongQueue) Processing() bool {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	return sq.processing
}

func (sq *SongQueue) SetProcessing(p bool) {
	sq.mu.Lock()
	defer sq.mu.Unlock()
	sq.processing = p
}

// ─────────────────────────────────────────────
//  GLOBAL STATE & PERSISTENCE
// ─────────────────────────────────────────────

var (
	live      = NewLiveConfig()
	songs     = &SongHistory{}
	songQueue = NewSongQueue()
	watcher   *CS2ChatWatcher

	baseDir           string
	configDir         string
	presetsFile       string
	configFile        string
	triggersFile      string
	banwordsFile      string
	queueSettingsFile string
	historyFile       string
	cfgPath           string

	cfgWriteMu     sync.Mutex
	lastWrittenCfg string
)

func initPaths() {
	cwd, err := os.Getwd()
	if err == nil {
		baseDir = cwd
	} else {
		exe, _ := os.Executable()
		baseDir = filepath.Dir(exe)
	}
	configDir = filepath.Join(baseDir, "configs")
	_ = os.MkdirAll(configDir, 0755)

	presetsFile = filepath.Join(configDir, "nowplaying_presets.json")
	configFile = filepath.Join(configDir, "nowplaying_config.json")
	triggersFile = filepath.Join(configDir, "nowplaying_triggers.json")
	banwordsFile = filepath.Join(configDir, "nowplaying_banwords.json")
	queueSettingsFile = filepath.Join(configDir, "nowplaying_queue_settings.json")
	historyFile = filepath.Join(configDir, "nowplaying_history.txt")
}

func loadPresetsFile() map[string]ConfigData {
	out := make(map[string]ConfigData)
	data, err := os.ReadFile(presetsFile)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(data, &out)
	return out
}

func savePresetsFile(presets map[string]ConfigData) bool {
	data, err := json.MarshalIndent(presets, "", "  ")
	if err != nil {
		return false
	}
	if err := os.WriteFile(presetsFile, data, 0644); err != nil {
		logMsg("ERROR", fmt.Sprintf("Could not save presets: %v", err))
		return false
	}
	return true
}

func loadConfigFile() *ConfigData {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil
	}
	var cfg ConfigData
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	return &cfg
}

func saveConfigFile() {
	data, err := json.MarshalIndent(live.ToDict(), "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(configFile, data, 0644); err != nil {
		logMsg("ERROR", fmt.Sprintf("Could not save config state: %v", err))
	}
}

func loadTriggersFile() []Trigger {
	data, err := os.ReadFile(triggersFile)
	if err != nil {
		return nil
	}
	var list []Trigger
	if err := json.Unmarshal(data, &list); err != nil {
		return nil
	}
	return list
}

func saveTriggersFile() {
	if watcher == nil {
		return
	}
	data, err := json.MarshalIndent(watcher.GetTriggers(), "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(triggersFile, data, 0644); err != nil {
		logMsg("ERROR", fmt.Sprintf("Could not save triggers: %v", err))
	}
}

func loadBanwordsFile() []string {
	data, err := os.ReadFile(banwordsFile)
	if err != nil {
		return nil
	}
	var words []string
	if err := json.Unmarshal(data, &words); err != nil {
		return nil
	}
	return words
}

func saveBanwordsFile() {
	if watcher == nil {
		return
	}
	data, err := json.MarshalIndent(watcher.GetBanWords(), "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(banwordsFile, data, 0644); err != nil {
		logMsg("ERROR", fmt.Sprintf("Could not save ban words: %v", err))
	}
}

func loadQueueSettingsFile() *QueueSettings {
	data, err := os.ReadFile(queueSettingsFile)
	if err != nil {
		return nil
	}
	var s QueueSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil
	}
	return &s
}

func saveQueueSettingsFile() {
	data, err := json.MarshalIndent(songQueue.GetSettings(), "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(queueSettingsFile, data, 0644); err != nil {
		logMsg("ERROR", fmt.Sprintf("Could not save queue settings: %v", err))
	}
}

// ─────────────────────────────────────────────
//  LOGGING & TERMINAL COLORS
// ─────────────────────────────────────────────

const (
	cReset  = "\033[0m"
	cGreen  = "\033[92m"
	cYellow = "\033[93m"
	cRed    = "\033[91m"
	cCyan   = "\033[96m"
	cBold   = "\033[1m"
	cDim    = "\033[2m"
)

func logMsg(level, msg string) {
	ts := time.Now().Format("15:04:05")
	color, icon := cReset, "."
	switch level {
	case "INFO":
		color, icon = cCyan, "i"
	case "OK":
		color, icon = cGreen, "v"
	case "WARN":
		color, icon = cYellow, "!"
	case "ERROR":
		color, icon = cRed, "x"
	case "MUSIC":
		color, icon = cBold+cGreen, "#"
	case "FILE":
		color, icon = cCyan, ">"
	case "CFG":
		color, icon = cYellow, "*"
	case "QUEUE":
		color, icon = cCyan, "q"
	}
	fmt.Printf("%s[%s]%s %s[%s] %s%s\n", cDim, ts, cReset, color, icon, msg, cReset)
}

// ─────────────────────────────────────────────
//  CS2 PATH DETECTION & CFG WRITING
// ─────────────────────────────────────────────

var vdfPathRe = regexp.MustCompile(`"path"\s+"([^"]+)"`)

func queryWindowsSteamPath() []string {
	if runtime.GOOS != "windows" {
		return nil
	}
	var roots []string
	queries := [][]string{
		{"reg", "query", `HKLM\SOFTWARE\WOW6432Node\Valve\Steam`, "/v", "InstallPath"},
		{"reg", "query", `HKCU\Software\Valve\Steam`, "/v", "SteamPath"},
	}
	for _, q := range queries {
		out, err := exec.Command(q[0], q[1:]...).Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			idx := strings.Index(line, "REG_SZ")
			if idx != -1 {
				val := strings.TrimSpace(line[idx+len("REG_SZ"):])
				val = filepath.FromSlash(val)
				if val != "" {
					roots = append(roots, val)
				}
			}
		}
	}
	return roots
}

func iterSteamLibraries(roots []string) []string {
	seen := make(map[string]struct{})
	var libraries []string

	addLib := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		info, err := os.Stat(p)
		if err != nil || !info.IsDir() {
			return
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		key := strings.ToLower(abs)
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			libraries = append(libraries, p)
		}
	}

	for _, r := range roots {
		addLib(r)
		vdf := filepath.Join(r, "steamapps", "libraryfolders.vdf")
		data, err := os.ReadFile(vdf)
		if err != nil {
			continue
		}
		for _, m := range vdfPathRe.FindAllStringSubmatch(string(data), -1) {
			libPath := strings.ReplaceAll(m[1], `\\`, `\`)
			addLib(libPath)
		}
	}
	return libraries
}

func findCS2CfgPath() string {
	rel := filepath.Join("steamapps", "common", "Counter-Strike Global Offensive", "game", "csgo", "cfg")
	var candidateRoots []string
	if runtime.GOOS == "windows" {
		candidateRoots = append(candidateRoots, queryWindowsSteamPath()...)
		candidateRoots = append(candidateRoots,
			`C:\Program Files (x86)\Steam`,
			`C:\Program Files\Steam`,
			`D:\Steam`,
			`E:\Steam`,
		)
	} else {
		home, _ := os.UserHomeDir()
		candidateRoots = append(candidateRoots,
			filepath.Join(home, ".steam", "steam"),
			filepath.Join(home, ".local", "share", "Steam"),
		)
	}

	for _, lib := range iterSteamLibraries(candidateRoots) {
		p := filepath.Join(lib, rel)
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	return ""
}

func resolveCfgPath() string {
	if cfgDir := findCS2CfgPath(); cfgDir != "" {
		p := filepath.Join(cfgDir, "nowplaying.cfg")
		logMsg("OK", "Found CS2 cfg folder: "+cfgDir)
		return p
	}
	logMsg("WARN", "Could not auto-detect CS2 cfg folder.")
	logMsg("INFO", "Enter the path to the cfg folder manually:")
	fmt.Print("  > ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	userInput := strings.Trim(strings.TrimSpace(line), `"'`)
	return filepath.Join(userInput, "nowplaying.cfg")
}

func writeCfg(title, artist, currentTime, duration, requester string) bool {
	content := live.GetCommand(title, artist, currentTime, duration, requester) + "\n"
	cfgWriteMu.Lock()
	defer cfgWriteMu.Unlock()

	if content == lastWrittenCfg && cfgPath != "" {
		if _, err := os.Stat(cfgPath); err == nil {
			return true
		}
	}
	if parent := filepath.Dir(cfgPath); parent != "" {
		_ = os.MkdirAll(parent, 0755)
	}
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		logMsg("ERROR", fmt.Sprintf("Could not write CFG file: %v", err))
		return false
	}
	lastWrittenCfg = content
	logMsg("FILE", strings.TrimSpace(content))
	return true
}

func writeCfgCurrent(placeholder bool) bool {
	title, artist, curTime, duration, requester := songs.Snapshot()
	if title == "" {
		if !placeholder {
			return false
		}
		title = "Song Title"
	}
	if artist == "" && placeholder {
		artist = "Artist"
	}
	if curTime == "" && placeholder {
		curTime = "0:00"
	}
	if duration == "" && placeholder {
		duration = "0:00"
	}
	if requester == "" && placeholder {
		requester = "Player"
	}
	return writeCfg(title, artist, curTime, duration, requester)
}

func logToHistory(title, artist string) {
	f, err := os.OpenFile(historyFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	entry := title
	if artist != "" {
		entry = artist + " - " + title
	}
	_, _ = fmt.Fprintf(f, "[%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), entry)
}

// ─────────────────────────────────────────────
//  DURATION PARSING & YOUTUBE SEARCH
// ─────────────────────────────────────────────

// parseDurationSeconds parses "3:45" or "1:02:30" into seconds.
func parseDurationSeconds(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	parts := strings.Split(s, ":")
	total := 0
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 {
			return 0
		}
		total = total*60 + n
	}
	return total
}

var (
	ytVideoBlockRe = regexp.MustCompile(`"videoRenderer":\{"videoId":"([a-zA-Z0-9_-]{11})"(?:.*?"lengthText":\{(?:.*?"simpleText":"([0-9:]+)")?)?`)
	ytDirectIDRe   = regexp.MustCompile(`(?:youtube\.com/watch\?v=|youtu\.be/|youtube\.com/shorts/)([a-zA-Z0-9_-]{11})`)
	httpClient     = &http.Client{Timeout: 6 * time.Second}
)

type YouTubeCandidate struct {
	VideoID     string
	DurationStr string
	DurationSec int
}

// searchYouTubeVideoWithLimit finds the first YouTube video matching query that fits within maxDurationSec (if > 0).
func searchYouTubeVideoWithLimit(query string, maxDurationSec int) (YouTubeCandidate, bool) {
	query = strings.TrimSpace(query)
	if query == "" {
		return YouTubeCandidate{}, false
	}
	if m := ytDirectIDRe.FindStringSubmatch(query); len(m) == 2 {
		return YouTubeCandidate{VideoID: m[1]}, false
	}

	searchURL := "https://www.youtube.com/results?search_query=" + url.QueryEscape(query)
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return YouTubeCandidate{}, false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := httpClient.Do(req)
	if err != nil {
		logMsg("WARN", fmt.Sprintf("YouTube search failed for %q: %v", query, err))
		return YouTubeCandidate{}, false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return YouTubeCandidate{}, false
	}

	text := string(body)
	// Split by "videoRenderer":{"videoId":" to inspect each result chunk safely
	chunks := strings.Split(text, `"videoRenderer":{"videoId":"`)
	if len(chunks) <= 1 {
		return YouTubeCandidate{}, false
	}

	lenRe := regexp.MustCompile(`"lengthText":\{[^}]*"simpleText":"([0-9:]+)"`)
	foundOverLimit := false

	for _, chunk := range chunks[1:] {
		if len(chunk) < 11 {
			continue
		}
		vid := chunk[:11]
		// Ensure valid 11-char ID followed by quote
		if len(chunk) > 11 && chunk[11] != '"' {
			continue
		}
		// Inspect up to 4000 bytes of this videoRenderer object for lengthText
		window := chunk
		if len(window) > 4000 {
			window = window[:4000]
		}
		durStr := ""
		durSec := 0
		if lm := lenRe.FindStringSubmatch(window); len(lm) == 2 {
			durStr = lm[1]
			durSec = parseDurationSeconds(durStr)
		}

		if maxDurationSec > 0 && durSec > maxDurationSec {
			foundOverLimit = true
			continue
		}
		return YouTubeCandidate{
			VideoID:     vid,
			DurationStr: durStr,
			DurationSec: durSec,
		}, false
	}

	return YouTubeCandidate{}, foundOverLimit
}

func searchYouTubeVideo(query string) string {
	settings := songQueue.GetSettings()
	maxSec := settings.MaxDurationMin * 60
	cand, _ := searchYouTubeVideoWithLimit(query, maxSec)
	return cand.VideoID
}

// ─────────────────────────────────────────────
//  DISPATCH & QUEUE ORCHESTRATION
// ─────────────────────────────────────────────

func splitArtistTitle(query string) (artist, title string) {
	if parts := strings.SplitN(query, " - ", 2); len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return "", strings.TrimSpace(query)
}

func dispatchQueuedSongToPlayer(song QueuedSong) {
	songQueue.SetProcessing(true)

	query := strings.TrimSpace(song.Query)
	if query == "" {
		if song.Artist != "" {
			query = song.Artist + " - " + song.Title
		} else {
			query = song.Title
		}
	}

	songs.SetSong(song.Title, song.Artist, "0:00", song.Duration, song.Requester)
	writeCfgCurrent(false)

	if watcher != nil {
		playEvt := ChatEvent{
			ID:           shortID(),
			Timestamp:    time.Now().Format("2006-01-02 - 15:04:05"),
			Player:       song.Requester,
			Message:      query,
			RawLine:      query,
			TriggerID:    "queue",
			TriggerLabel: "Queue Play",
			SearchQuery:  query,
			Status:       "Playing now",
			VideoID:      song.VideoID,
			ReceivedAt:   float64(time.Now().UnixNano()) / 1e9,
		}
		watcher.PushPlayOnly(playEvt)
		logMsg("QUEUE", fmt.Sprintf("Dispatched playback: %s (requested by %s)", query, song.Requester))
	}

	go func() {
		time.Sleep(800 * time.Millisecond)
		songQueue.SetProcessing(false)
	}()
}

// advanceQueueIfPossible pops the next song in queue and plays it.
func advanceQueueIfPossible(reason string) *QueuedSong {
	nextSong := songQueue.GetNext()
	if nextSong == nil {
		songs.MarkEnded()
		return nil
	}
	logMsg("QUEUE", fmt.Sprintf("Auto-advancing queue (%s): %s - %s", reason, nextSong.Artist, nextSong.Title))
	dispatchQueuedSongToPlayer(*nextSong)
	return nextSong
}

// handleChatTriggerEvent processes a matched chat trigger according to QueueSettings.
func handleChatTriggerEvent(evt *ChatEvent) {
	query := strings.TrimSpace(evt.SearchQuery)
	if query == "" {
		query = strings.TrimSpace(evt.Message)
	}
	if query == "" {
		evt.Status = "Ignored: empty query"
		return
	}

	// 1. Check cooldowns
	if ok, reason := songQueue.CheckCooldowns(evt.Player); !ok {
		evt.Status = "Rejected: " + reason
		logMsg("WARN", fmt.Sprintf("Ignored request %q from %s (%s)", query, evt.Player, reason))
		return
	}

	artist, title := splitArtistTitle(query)

	// 2. Check duplicate before YouTube lookup
	if songQueue.IsDuplicate(query, title, artist, "") {
		evt.Status = "Rejected: duplicate song"
		logMsg("WARN", fmt.Sprintf("Ignored duplicate request %q from %s", query, evt.Player))
		return
	}

	settings := songQueue.GetSettings()
	requester := evt.Player
	if requester == "" {
		requester = "chat"
	}

	item := QueuedSong{
		ID:        shortID(),
		Title:     title,
		Artist:    artist,
		Query:     query,
		Requester: requester,
		Source:    "chat:" + requester,
		AddedAt:   time.Now().Format("2006-01-02 15:04:05"),
	}

	songQueue.RecordTriggerTime(evt.Player)

	switch settings.PlaybackMode {
	case "instant":
		evt.Status = "Playing now (Instant)"
		dispatchQueuedSongToPlayer(item)

	case "manual":
		pos, rejectReason := songQueue.AddWithRules(item, true)
		if rejectReason != "" {
			evt.Status = "Rejected: " + rejectReason
			logMsg("WARN", fmt.Sprintf("Could not queue %q from %s: %s", query, evt.Player, rejectReason))
			return
		}
		evt.Status = fmt.Sprintf("Queued (#%d · awaiting approval)", pos)
		logMsg("QUEUE", fmt.Sprintf("Queued (#%d, manual mode) from %s: %s", pos, evt.Player, query))

	default: // "auto_queue"
		if !songs.IsCurrentlyPlaying() && songQueue.IsEmpty() {
			evt.Status = "Playing now (Idle)"
			dispatchQueuedSongToPlayer(item)
		} else {
			pos, rejectReason := songQueue.AddWithRules(item, true)
			if rejectReason != "" {
				evt.Status = "Rejected: " + rejectReason
				logMsg("WARN", fmt.Sprintf("Could not queue %q from %s: %s", query, evt.Player, rejectReason))
				return
			}
			evt.Status = fmt.Sprintf("Queued (#%d)", pos)
			logMsg("QUEUE", fmt.Sprintf("Queued (#%d) from %s: %s", pos, evt.Player, query))
		}
	}
}

// ─────────────────────────────────────────────
//  BROWSER AUTO-OPEN
// ─────────────────────────────────────────────

func openBrowser(targetURL string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
	case "darwin":
		cmd = exec.Command("open", targetURL)
	default:
		cmd = exec.Command("xdg-open", targetURL)
	}
	_ = cmd.Start()
}

// ─────────────────────────────────────────────
//  HTTP HELPERS & HANDLERS
// ─────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

func queueStatePayload() map[string]any {
	_, _, _, _, requester := songs.Snapshot()
	return map[string]any{
		"queue":             songQueue.GetQueue(),
		"size":              songQueue.Size(),
		"processing":        songQueue.Processing(),
		"settings":          songQueue.GetSettings(),
		"is_playing":        songs.IsCurrentlyPlaying(),
		"current_requester": requester,
	}
}

func buildMux() http.Handler {
	mux := http.NewServeMux()

	// Static files from embedded FS
	staticSub, _ := fs.Sub(embeddedAssets, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Index panel
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		data, err := embeddedAssets.ReadFile("templates/index.html")
		if err != nil {
			http.Error(w, "Template not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})

	// Consolidated Panel State
	mux.HandleFunc("GET /panel/state", func(w http.ResponseWriter, r *http.Request) {
		var wStatus map[string]any
		var events []ChatEvent
		if watcher == nil {
			wStatus = map[string]any{"running": false, "reason": "not initialized", "log_path": "", "log_exists": false}
			events = []ChatEvent{}
		} else {
			wStatus = map[string]any{
				"running":    watcher.IsRunning(),
				"log_path":   watcher.LogPath,
				"log_exists": watcher.LogExists(),
			}
			events = watcher.GetRecentEvents(30)
		}
		lastTitle, lastArtist, lastTime, lastDur, lastReq := songs.Snapshot()
		writeJSON(w, http.StatusOK, map[string]any{
			"last_song":      lastTitle,
			"last_artist":    lastArtist,
			"last_time":      lastTime,
			"last_duration":  lastDur,
			"last_requester": lastReq,
			"is_playing":     songs.IsCurrentlyPlaying(),
			"watcher":        wStatus,
			"events":         events,
			"queue":          queueStatePayload(),
		})
	})

	// Config GET / POST
	mux.HandleFunc("GET /config", func(w http.ResponseWriter, r *http.Request) {
		cfg := live.ToDict()
		lastTitle, lastArtist, lastTime, lastDur, lastReq := songs.Snapshot()
		writeJSON(w, http.StatusOK, map[string]any{
			"blocks":          cfg.Blocks,
			"use_colors":      cfg.UseColors,
			"chat_command":    cfg.ChatCommand,
			"block_separator": cfg.BlockSeparator,
			"last_song":       lastTitle,
			"last_artist":     lastArtist,
			"last_time":       lastTime,
			"last_duration":   lastDur,
			"last_requester":  lastReq,
			"queue_settings":  songQueue.GetSettings(),
		})
	})

	mux.HandleFunc("POST /config", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Blocks         *[]Block `json:"blocks"`
			UseColors      *bool    `json:"use_colors"`
			ChatCommand    *string  `json:"chat_command"`
			BlockSeparator *string  `json:"block_separator"`
		}
		if err := readJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
			return
		}
		live.Update(req.Blocks, req.UseColors, req.ChatCommand, req.BlockSeparator)
		cfg := live.ToDict()
		logMsg("CFG", fmt.Sprintf("Config updated (%d blocks, cmd=%s)", len(cfg.Blocks), cfg.ChatCommand))

		lastTitle, _, _, _, _ := songs.Snapshot()
		writeCfgCurrent(true)
		if lastTitle != "" {
			logMsg("OK", "CFG written (real data).\n")
		} else {
			logMsg("OK", "CFG written (placeholder — waiting for SoundCloud/YouTube).\n")
		}
		saveConfigFile()
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":               true,
			"used_placeholder": lastTitle == "",
		})
	})

	// Time POST
	mux.HandleFunc("POST /time", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			CurrentTime string `json:"current_time"`
			Duration    string `json:"duration"`
		}
		if err := readJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
			return
		}
		cur := strings.TrimSpace(req.CurrentTime)
		dur := strings.TrimSpace(req.Duration)
		if cur == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "empty time"})
			return
		}
		songs.UpdateTime(cur, dur)
		needTime := live.HasTimeBlocks()
		lastTitle, _, _, _, _ := songs.Snapshot()
		if needTime && lastTitle != "" {
			writeCfgCurrent(false)
		}

		// Enforce MaxDurationMin limit during playback
		qSettings := songQueue.GetSettings()
		maxSec := qSettings.MaxDurationMin * 60
		autoSkipped := false
		if maxSec > 0 {
			curSec := parseDurationSeconds(cur)
			if curSec >= maxSec && songs.TryMarkAutoSkip() {
				logMsg("WARN", fmt.Sprintf("Song reached max duration limit (%d min) — advancing queue", qSettings.MaxDurationMin))
				if qSettings.PlaybackMode != "manual" && !songQueue.IsEmpty() {
					advanceQueueIfPossible("max duration reached")
					autoSkipped = true
				}
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":           "ok",
			"need_time":        needTime,
			"max_duration_sec": maxSec,
			"auto_skipped":     autoSkipped,
		})
	})

	// Song POST
	mux.HandleFunc("POST /song", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Title  string `json:"title"`
			Artist string `json:"artist"`
		}
		if err := readJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
			return
		}
		title := strings.TrimSpace(req.Title)
		artist := strings.TrimSpace(req.Artist)
		if title == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "empty title"})
			return
		}
		needTime := live.HasTimeBlocks()
		maxSec := songQueue.GetSettings().MaxDurationMin * 60
		if !songs.IsNew(title, artist) {
			logMsg("INFO", fmt.Sprintf("Duplicate — skipped: %q - %q", artist, title))
			writeJSON(w, http.StatusOK, map[string]any{"status": "duplicate", "need_time": needTime, "max_duration_sec": maxSec})
			return
		}
		display := title
		if artist != "" {
			display = artist + " - " + title
		}
		logMsg("MUSIC", cBold+display+cReset)
		logToHistory(title, artist)

		if writeCfgCurrent(false) {
			logMsg("OK", "CFG updated! Press MOUSE4/MOUSE5 in CS2 to send.\n")
			writeJSON(w, http.StatusOK, map[string]any{"status": "cfg_updated", "need_time": needTime, "max_duration_sec": maxSec})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"status": "write_error", "need_time": needTime, "max_duration_sec": maxSec})
		}
	})

	// Presets
	mux.HandleFunc("GET /presets", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, loadPresetsFile())
	})

	mux.HandleFunc("POST /presets", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string `json:"name"`
		}
		if err := readJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "empty name"})
			return
		}
		presets := loadPresetsFile()
		presets[name] = live.ToDict()
		if savePresetsFile(presets) {
			logMsg("CFG", fmt.Sprintf("Saved preset: %q", name))
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": name})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "write failed"})
		}
	})

	mux.HandleFunc("POST /presets/import", func(w http.ResponseWriter, r *http.Request) {
		var incoming map[string]struct {
			Blocks         *[]Block `json:"blocks"`
			UseColors      bool     `json:"use_colors"`
			ChatCommand    string   `json:"chat_command"`
			BlockSeparator string   `json:"block_separator"`
		}
		if err := readJSON(r, &incoming); err != nil || incoming == nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
			return
		}
		presets := loadPresetsFile()
		imported := 0
		for name, cfg := range incoming {
			if cfg.Blocks != nil {
				cmd := cfg.ChatCommand
				if cmd != "say_team" {
					cmd = "say"
				}
				sep := cfg.BlockSeparator
				if sep == "" {
					sep = " "
				}
				presets[name] = ConfigData{
					Blocks:         sanitizeBlocks(*cfg.Blocks),
					UseColors:      cfg.UseColors,
					ChatCommand:    cmd,
					BlockSeparator: sep,
				}
				imported++
			}
		}
		savePresetsFile(presets)
		logMsg("CFG", fmt.Sprintf("Imported %d preset(s)", imported))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "imported": imported})
	})

	mux.HandleFunc("DELETE /presets/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		presets := loadPresetsFile()
		if _, exists := presets[name]; !exists {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
			return
		}
		delete(presets, name)
		savePresetsFile(presets)
		logMsg("CFG", fmt.Sprintf("Deleted preset: %q", name))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	mux.HandleFunc("POST /presets/{name}/load", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		presets := loadPresetsFile()
		preset, exists := presets[name]
		if !exists {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
			return
		}
		live.Update(&preset.Blocks, &preset.UseColors, &preset.ChatCommand, &preset.BlockSeparator)
		logMsg("CFG", fmt.Sprintf("Loaded preset: %q", name))
		writeCfgCurrent(true)
		saveConfigFile()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "config": live.ToDict()})
	})

	// Watcher endpoints
	mux.HandleFunc("GET /watcher/status", func(w http.ResponseWriter, r *http.Request) {
		if watcher == nil {
			writeJSON(w, http.StatusOK, map[string]any{"running": false, "reason": "not initialized"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"running":    watcher.IsRunning(),
			"log_path":   watcher.LogPath,
			"log_exists": watcher.LogExists(),
		})
	})

	mux.HandleFunc("GET /watcher/triggers", func(w http.ResponseWriter, r *http.Request) {
		if watcher == nil {
			writeJSON(w, http.StatusOK, []Trigger{})
			return
		}
		writeJSON(w, http.StatusOK, watcher.GetTriggers())
	})

	mux.HandleFunc("POST /watcher/triggers", func(w http.ResponseWriter, r *http.Request) {
		if watcher == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "watcher not running"})
			return
		}
		var req struct {
			Label       string `json:"label"`
			Pattern     string `json:"pattern"`
			Regex       string `json:"regex"`
			StripPrefix string `json:"strip_prefix"`
			Enabled     *bool  `json:"enabled"`
		}
		if err := readJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
			return
		}
		label := strings.TrimSpace(req.Label)
		pattern := strings.TrimSpace(req.Pattern)
		regex := strings.TrimSpace(req.Regex)
		stripPrefix := strings.TrimSpace(req.StripPrefix)
		if label == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "label is required"})
			return
		}
		if pattern == "" && regex == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "pattern or regex is required"})
			return
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		trig := watcher.AddTrigger(Trigger{
			ID:          shortID(),
			Label:       label,
			Pattern:     pattern,
			Regex:       regex,
			Enabled:     enabled,
			StripPrefix: stripPrefix,
		})
		saveTriggersFile()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "trigger": trig})
	})

	mux.HandleFunc("DELETE /watcher/triggers/{trigger_id}", func(w http.ResponseWriter, r *http.Request) {
		if watcher == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "watcher not running"})
			return
		}
		id := r.PathValue("trigger_id")
		if !watcher.RemoveTrigger(id) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
			return
		}
		saveTriggersFile()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "triggers": watcher.GetTriggers()})
	})

	mux.HandleFunc("PATCH /watcher/triggers/{trigger_id}", func(w http.ResponseWriter, r *http.Request) {
		if watcher == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "watcher not running"})
			return
		}
		id := r.PathValue("trigger_id")
		var updates map[string]any
		if err := readJSON(r, &updates); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
			return
		}
		updated := watcher.UpdateTrigger(id, updates)
		if updated == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
			return
		}
		saveTriggersFile()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "trigger": updated, "triggers": watcher.GetTriggers()})
	})

	mux.HandleFunc("GET /watcher/events", func(w http.ResponseWriter, r *http.Request) {
		if watcher == nil {
			writeJSON(w, http.StatusOK, []ChatEvent{})
			return
		}
		limit := 50
		if lStr := r.URL.Query().Get("limit"); lStr != "" {
			if n, err := strconv.Atoi(lStr); err == nil && n > 0 {
				limit = n
			}
		}
		if limit > 200 {
			limit = 200
		}
		// If ?mode=ui is passed, return all UI log events; otherwise return playback-dispatched events for userscripts
		if r.URL.Query().Get("mode") == "ui" {
			writeJSON(w, http.StatusOK, watcher.GetRecentEvents(limit))
			return
		}
		writeJSON(w, http.StatusOK, watcher.GetPlayEvents(limit))
	})

	// Banwords
	mux.HandleFunc("GET /watcher/banwords", func(w http.ResponseWriter, r *http.Request) {
		if watcher == nil {
			writeJSON(w, http.StatusOK, []string{})
			return
		}
		writeJSON(w, http.StatusOK, watcher.GetBanWords())
	})

	mux.HandleFunc("POST /watcher/banwords", func(w http.ResponseWriter, r *http.Request) {
		if watcher == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "watcher not running"})
			return
		}
		var req struct {
			Word string `json:"word"`
		}
		if err := readJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
			return
		}
		word := strings.TrimSpace(req.Word)
		if word == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "empty word"})
			return
		}
		added := watcher.AddBanWord(word)
		if added {
			saveBanwordsFile()
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "word": strings.ToLower(word), "added": added})
	})

	mux.HandleFunc("DELETE /watcher/banwords/{word}", func(w http.ResponseWriter, r *http.Request) {
		if watcher == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "watcher not running"})
			return
		}
		word := r.PathValue("word")
		if !watcher.RemoveBanWord(word) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
			return
		}
		saveBanwordsFile()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	// ── Queue & Queue Settings ───────────────────

	mux.HandleFunc("GET /queue", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, queueStatePayload())
	})

	mux.HandleFunc("GET /queue/settings", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, songQueue.GetSettings())
	})

	mux.HandleFunc("POST /queue/settings", func(w http.ResponseWriter, r *http.Request) {
		var incoming QueueSettings
		if err := readJSON(r, &incoming); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
			return
		}
		updated := songQueue.UpdateSettings(incoming)
		saveQueueSettingsFile()
		logMsg("CFG", fmt.Sprintf("Queue settings updated (mode=%s, maxSize=%d, maxDur=%dm)",
			updated.PlaybackMode, updated.MaxQueueSize, updated.MaxDurationMin))
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":       true,
			"settings": updated,
			"queue":    queueStatePayload(),
		})
	})

	mux.HandleFunc("POST /queue", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string `json:"query"`
			Title     string `json:"title"`
			Artist    string `json:"artist"`
			Source    string `json:"source"`
			Requester string `json:"requester"`
			PlayNow   bool   `json:"play_now"`
		}
		if err := readJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
			return
		}
		query := strings.TrimSpace(req.Query)
		title := strings.TrimSpace(req.Title)
		artist := strings.TrimSpace(req.Artist)
		if query != "" && title == "" {
			artist, title = splitArtistTitle(query)
		}
		if title == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "title or query is required"})
			return
		}
		if query == "" {
			if artist != "" {
				query = artist + " - " + title
			} else {
				query = title
			}
		}
		source := strings.TrimSpace(req.Source)
		if source == "" {
			source = "panel"
		}
		requester := strings.TrimSpace(req.Requester)
		if requester == "" {
			requester = source
		}

		songItem := QueuedSong{
			ID:        shortID(),
			Title:     title,
			Artist:    artist,
			Query:     query,
			Requester: requester,
			Source:    source,
			AddedAt:   time.Now().Format("2006-01-02 15:04:05"),
		}

		if req.PlayNow {
			dispatchQueuedSongToPlayer(songItem)
			resp := queueStatePayload()
			resp["ok"] = true
			resp["played_immediately"] = true
			writeJSON(w, http.StatusOK, resp)
			return
		}

		if songQueue.IsDuplicate(query, title, artist, "") {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "duplicate song is already queued or playing"})
			return
		}

		pos, errReason := songQueue.AddWithRules(songItem, false)
		if errReason != "" {
			writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": errReason})
			return
		}

		logMsg("QUEUE", fmt.Sprintf("Added to queue (#%d): %s", pos, query))

		// In auto_queue mode, if nothing is currently playing and this is the only song, start it immediately
		settings := songQueue.GetSettings()
		if settings.PlaybackMode == "auto_queue" && !songs.IsCurrentlyPlaying() && songQueue.Size() == 1 {
			advanceQueueIfPossible("idle auto-start")
		}

		resp := queueStatePayload()
		resp["ok"] = true
		writeJSON(w, http.StatusOK, resp)
	})

	mux.HandleFunc("DELETE /queue/{song_id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("song_id")
		if songQueue.Remove(id) {
			logMsg("QUEUE", "Removed song from queue: "+id)
			resp := queueStatePayload()
			resp["ok"] = true
			writeJSON(w, http.StatusOK, resp)
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "song not found"})
	})

	mux.HandleFunc("POST /queue/clear", func(w http.ResponseWriter, r *http.Request) {
		songQueue.Clear()
		logMsg("QUEUE", "Queue cleared")
		resp := queueStatePayload()
		resp["ok"] = true
		writeJSON(w, http.StatusOK, resp)
	})

	mux.HandleFunc("POST /queue/{song_id}/move", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("song_id")
		var req struct {
			Position int `json:"position"`
		}
		if err := readJSON(r, &req); err != nil || req.Position < 1 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid position"})
			return
		}
		if songQueue.Move(id, req.Position) {
			resp := queueStatePayload()
			resp["ok"] = true
			writeJSON(w, http.StatusOK, resp)
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "song not found or invalid position"})
	})

	mux.HandleFunc("POST /queue/{song_id}/play", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("song_id")
		picked := songQueue.PopByID(id)
		if picked == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "song not found"})
			return
		}
		dispatchQueuedSongToPlayer(*picked)
		resp := queueStatePayload()
		resp["ok"] = true
		resp["song"] = picked
		writeJSON(w, http.StatusOK, resp)
	})

	mux.HandleFunc("POST /queue/next", func(w http.ResponseWriter, r *http.Request) {
		nextSong := advanceQueueIfPossible("manual next")
		if nextSong == nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "queue is empty"})
			return
		}
		resp := queueStatePayload()
		resp["ok"] = true
		resp["song"] = nextSong
		writeJSON(w, http.StatusOK, resp)
	})

	mux.HandleFunc("POST /queue/ended", func(w http.ResponseWriter, r *http.Request) {
		songs.MarkEnded()
		settings := songQueue.GetSettings()
		var nextSong *QueuedSong
		if settings.PlaybackMode != "manual" && !songQueue.IsEmpty() {
			nextSong = advanceQueueIfPossible("track ended")
		}
		resp := queueStatePayload()
		resp["ok"] = true
		resp["advanced"] = nextSong != nil
		resp["song"] = nextSong
		writeJSON(w, http.StatusOK, resp)
	})

	// YouTube Search (duration-aware)
	mux.HandleFunc("GET /youtube/search", func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing query parameter q"})
			return
		}
		settings := songQueue.GetSettings()
		maxSec := settings.MaxDurationMin * 60
		cand, overLimit := searchYouTubeVideoWithLimit(q, maxSec)
		if cand.VideoID == "" {
			if overLimit {
				logMsg("WARN", fmt.Sprintf("YouTube results for %q exceeded max duration (%d min)", q, settings.MaxDurationMin))
				// Auto-advance queue if this song was too long
				if !songQueue.IsEmpty() && settings.PlaybackMode != "manual" {
					advanceQueueIfPossible("skipped video exceeding max duration")
				}
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
					"error": fmt.Sprintf("video exceeds max duration (%d min)", settings.MaxDurationMin),
					"query": q,
				})
				return
			}
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "no video found", "query": q})
			return
		}
		watchURL := "https://www.youtube.com/watch?v=" + cand.VideoID
		logMsg("MUSIC", fmt.Sprintf("YouTube resolved %q -> %s (%s)", q, watchURL, cand.DurationStr))
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":               true,
			"query":            q,
			"video_id":         cand.VideoID,
			"duration":         cand.DurationStr,
			"duration_sec":     cand.DurationSec,
			"max_duration_sec": maxSec,
			"url":              watchURL,
		})
	})

	// Handle CORS preflight OPTIONS for all endpoints
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// ─────────────────────────────────────────────
//  MAIN
// ─────────────────────────────────────────────

func main() {
	port := flag.Int("port", 9999, "HTTP server port")
	noBrowser := flag.Bool("no-browser", false, "Do not auto-open browser on startup")
	flag.Parse()

	initPaths()

	fmt.Printf("\n%s%s+------------------------------------------------------+\n", cCyan, cBold)
	fmt.Printf("|   CS2 Now Playing  .  YouTube / SoundCloud -> CFG    |\n")
	fmt.Printf("+------------------------------------------------------+%s\n\n", cReset)

	cfgPath = resolveCfgPath()
	fmt.Println()

	consoleLogPath := FindConsoleLog(cfgPath)
	watcher = NewCS2ChatWatcher(consoleLogPath)

	// Restore queue settings before starting watcher
	if savedQS := loadQueueSettingsFile(); savedQS != nil {
		songQueue.UpdateSettings(*savedQS)
		logMsg("OK", "Restored queue settings from "+queueSettingsFile)
	} else {
		watcher.SetChannelFilter(songQueue.GetSettings().ChatChannels)
	}

	watcher.Start()
	watcher.AddCallback(handleChatTriggerEvent)

	// Restore triggers (or create default !play trigger)
	if savedTriggers := loadTriggersFile(); len(savedTriggers) > 0 {
		for _, t := range savedTriggers {
			watcher.AddTrigger(t)
		}
		logMsg("OK", fmt.Sprintf("Restored %d trigger(s) from %s", len(savedTriggers), triggersFile))
	} else {
		watcher.AddTrigger(Trigger{
			ID:          "play_cmd",
			Label:       "Play Song (!play)",
			Pattern:     "!play",
			StripPrefix: "!play",
			Enabled:     true,
		})
		saveTriggersFile()
		logMsg("OK", "Created default '!play' chat trigger")
	}

	// Restore banwords
	if savedBanwords := loadBanwordsFile(); len(savedBanwords) > 0 {
		watcher.SetBanWords(savedBanwords)
		logMsg("OK", fmt.Sprintf("Restored %d ban word(s) from %s", len(savedBanwords), banwordsFile))
	}

	// Restore config state
	if savedCfg := loadConfigFile(); savedCfg != nil {
		live.Update(&savedCfg.Blocks, &savedCfg.UseColors, &savedCfg.ChatCommand, &savedCfg.BlockSeparator)
		logMsg("OK", "Restored config state from "+configFile)
	}

	panelURL := fmt.Sprintf("http://localhost:%d", *port)
	logMsg("INFO", "Server / Panel : "+panelURL)
	logMsg("INFO", "CFG file       : "+cfgPath)
	logMsg("INFO", "Console log    : "+consoleLogPath)
	logMsg("INFO", "History file   : "+historyFile)
	logMsg("INFO", "Presets file   : "+presetsFile)
	fmt.Println()
	logMsg("WARN", `CS2 bind       : bind "MOUSE4" "exec nowplaying.cfg"`)
	logMsg("WARN", `CS2 launch opt : -condebug -noclearlog  (required for chat watcher)`)
	fmt.Println()
	logMsg("INFO", "Waiting for data from YouTube / SoundCloud...\n")

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logMsg("ERROR", fmt.Sprintf("Failed to bind %s: %v", addr, err))
		os.Exit(1)
	}

	if !*noBrowser {
		go func() {
			time.Sleep(250 * time.Millisecond)
			logMsg("INFO", "Opening "+panelURL+" in your default browser...")
			openBrowser(panelURL)
		}()
	}

	if err := http.Serve(ln, buildMux()); err != nil {
		logMsg("ERROR", fmt.Sprintf("Server stopped: %v", err))
		os.Exit(1)
	}
}
