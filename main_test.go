package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReworkedQueueAndCustomizations(t *testing.T) {
	tmp := t.TempDir()
	baseDir = tmp
	configDir = filepath.Join(tmp, "configs")
	_ = os.MkdirAll(configDir, 0755)
	presetsFile = filepath.Join(configDir, "nowplaying_presets.json")
	configFile = filepath.Join(configDir, "nowplaying_config.json")
	triggersFile = filepath.Join(configDir, "nowplaying_triggers.json")
	banwordsFile = filepath.Join(configDir, "nowplaying_banwords.json")
	queueSettingsFile = filepath.Join(configDir, "nowplaying_queue_settings.json")
	historyFile = filepath.Join(configDir, "nowplaying_history.txt")
	cfgPath = filepath.Join(tmp, "nowplaying.cfg")

	logFile := filepath.Join(tmp, "console.log")
	_ = os.WriteFile(logFile, nil, 0644)

	live = NewLiveConfig()
	songs = &SongHistory{}
	songQueue = NewSongQueue()
	// Configure zero cooldowns for rapid unit test assertions
	songQueue.UpdateSettings(QueueSettings{
		PlaybackMode:         "auto_queue",
		CooldownSec:          0,
		PerPlayerCooldownSec: 0,
		MaxQueueSize:         5,
		MaxPerPlayer:         2,
		PreventDuplicates:    true,
		MaxDurationMin:       1, // 1 minute = 60 seconds limit
		ChatChannels:         "both",
	})

	watcher = NewCS2ChatWatcher(logFile)
	watcher.AddCallback(handleChatTriggerEvent)
	watcher.AddTrigger(Trigger{
		ID:          "play_cmd",
		Label:       "Play Song (!play)",
		Pattern:     "!play",
		StripPrefix: "!play",
		Enabled:     true,
	})

	mux := buildMux()

	// 1. Test embedded index.html and static assets
	for _, path := range []string{"/", "/static/app.js", "/static/style.css"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
			t.Fatalf("GET %s failed: status=%d len=%d", path, rec.Code, rec.Body.Len())
		}
	}

	// 2. First chat trigger while idle -> should play immediately in auto_queue mode
	f, _ := os.OpenFile(logFile, os.O_WRONLY|os.O_APPEND, 0644)
	_, _ = f.WriteString("10/28 16:00:01  [CT] Alice [DEAD]: !play Darude - Sandstorm\r\n")
	_ = f.Close()
	watcher.readNewLines()

	if !songs.IsCurrentlyPlaying() {
		t.Fatalf("expected first song to start playing immediately when idle")
	}
	if songQueue.Size() != 0 {
		t.Fatalf("expected queue size 0 after immediate idle play, got %d", songQueue.Size())
	}

	// 3. Second chat trigger while song 1 is playing -> should be added to queue (#1)
	f, _ = os.OpenFile(logFile, os.O_WRONLY|os.O_APPEND, 0644)
	_, _ = f.WriteString("10/28 16:00:02  [ALL] Bob: !play Linkin Park - Numb\r\n")
	_ = f.Close()
	watcher.readNewLines()

	if songQueue.Size() != 1 {
		t.Fatalf("expected second song to be queued (#1), got size %d", songQueue.Size())
	}

	// 4. Duplicate request -> should be rejected when PreventDuplicates is true
	f, _ = os.OpenFile(logFile, os.O_WRONLY|os.O_APPEND, 0644)
	_, _ = f.WriteString("10/28 16:00:03  [ALL] Charlie: !play Linkin Park - Numb\r\n")
	_ = f.Close()
	watcher.readNewLines()

	if songQueue.Size() != 1 {
		t.Fatalf("expected duplicate song to be rejected, queue size=%d", songQueue.Size())
	}
	uiEvts := watcher.GetRecentEvents(10)
	if !strings.Contains(strings.ToLower(uiEvts[0].Status), "duplicate") {
		t.Fatalf("expected duplicate status on newest UI event, got %q", uiEvts[0].Status)
	}

	// 5. Simulate current song exceeding MaxDurationMin (1 min = 60s) via POST /time -> should auto-skip to queued song!
	timeReq := httptest.NewRequest("POST", "/time", bytes.NewBufferString(`{"current_time":"1:05","duration":"3:30"}`))
	timeRec := httptest.NewRecorder()
	mux.ServeHTTP(timeRec, timeReq)
	if timeRec.Code != http.StatusOK {
		t.Fatalf("POST /time failed: %d", timeRec.Code)
	}
	var timeResp map[string]any
	_ = json.Unmarshal(timeRec.Body.Bytes(), &timeResp)
	if timeResp["auto_skipped"] != true {
		t.Fatalf("expected auto_skipped=true when exceeding max_duration_min, got %v", timeResp)
	}
	curTitle, curArtist, _, _, curReq := songs.Snapshot()
	if curTitle != "Numb" || curArtist != "Linkin Park" || curReq != "Bob" {
		t.Fatalf("expected auto-advance to Linkin Park - Numb (by Bob), got %q - %q (by %q)", curArtist, curTitle, curReq)
	}

	// 6. Test custom chat_command ("say_team"), block_separator (" | "), and "requester" block
	cfgBody := bytes.NewBufferString(`{
		"blocks": [
			{"id":"b1","type":"artist","value":"","color":"none","font":"normal"},
			{"id":"b2","type":"title","value":"","color":"none","font":"normal"},
			{"id":"b3","type":"requester","value":"","color":"none","font":"normal"}
		],
		"use_colors": false,
		"chat_command": "say_team",
		"block_separator": " | "
	}`)
	cfgReq := httptest.NewRequest("POST", "/config", cfgBody)
	cfgRec := httptest.NewRecorder()
	mux.ServeHTTP(cfgRec, cfgReq)
	if cfgRec.Code != http.StatusOK {
		t.Fatalf("POST /config failed: %d", cfgRec.Code)
	}
	rawCfg, _ := os.ReadFile(cfgPath)
	expectedCmd := `say_team "Linkin Park | Numb | Bob"`
	if strings.TrimSpace(string(rawCfg)) != expectedCmd {
		t.Fatalf("expected CFG %q, got %q", expectedCmd, strings.TrimSpace(string(rawCfg)))
	}
}
