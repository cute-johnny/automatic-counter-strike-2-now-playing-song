# CS2 Now Playing & Chat Triggers (Go Edition)

Automatically detects what you are listening to on **YouTube** or **SoundCloud**, lets you share it in CS2 chat with a single mouse button press, and allows you or your teammates to play songs on YouTube/SoundCloud directly from CS2 chat (`!play <song>`).

All web panel assets (`templates/index.html`, `static/style.css`, `static/app.js`) are **embedded directly inside the standalone Go binary** (`cs2nowplaying.exe`), which automatically opens `http://localhost:9999` in your default browser on launch.

---

## How It Works

### 1. Now Playing (`YouTube / SoundCloud → CS2 Chat`)
```text
YouTube or SoundCloud (browser)
  └─ Violentmonkey userscript scrapes current track & time
       └─ Sends track info to local Go server (localhost:9999)
            └─ Go server writes nowplaying.cfg into your CS2 cfg folder
                 └─ Press MOUSE4 in CS2 → message appears in chat
```

### 2. Chat Triggers (`CS2 Chat !play <song> → YouTube / SoundCloud`)
```text
Teammate or player types "!play <song>" in CS2 chat ([ALL], [T], [CT], [TEAM])
  └─ CS2 writes chat line to console.log (-condebug -noclearlog)
       └─ Go server watcher matches trigger & resolves YouTube videoId (no API key needed)
            └─ Browser userscript auto-plays the video & updates the Song Queue
```

---

## Requirements

| Requirement | Where to get it |
|---|---|
| **CS2 Now Playing Server** | Run `cs2nowplaying.exe` directly, or build from source with [Go 1.22+](https://go.dev/dl/) (`go build -o cs2nowplaying.exe .`) |
| **Violentmonkey** browser extension | https://violentmonkey.github.io/ |
| **Steam + Counter-Strike 2** | https://store.steampowered.com/app/730 |

---

## Userscripts Overview

Install the script(s) for the music platform you use into Violentmonkey:

| File | Platform | What it does |
|---|---|---|
| `youtube_nowplaying.js` | **YouTube** (`youtube.com`) | **All-in-one**: Scrapes YouTube track title, channel, and playback time for `nowplaying.cfg` **AND** listens for CS2 `!play <song>` chat triggers to automatically search & play videos on YouTube. |
| `soundcloudartistandsong.js` | **SoundCloud** (`soundcloud.com`) | Scrapes SoundCloud track title, artist, and playback time for `nowplaying.cfg`. |
| `cs2_chat_triggers.js` | **SoundCloud** (`soundcloud.com`) | Listens for CS2 chat triggers and searches/plays tracks on SoundCloud. |

---

## Step-by-Step Setup

### Step 1 — Install Violentmonkey & Userscript(s)

1. Install [Violentmonkey](https://violentmonkey.github.io/) for your browser (Chrome, Edge, Brave, or Firefox).
2. Click the Violentmonkey icon in your browser toolbar and select **`+` (Create a new script)**.
3. Delete the default template code in the editor.
4. Copy and paste the contents of:
   - **`youtube_nowplaying.js`** (recommended for YouTube `!play <song>` + Now Playing), and/or
   - **`soundcloudartistandsong.js`** (for SoundCloud Now Playing)
5. Press **Ctrl+S** to save.

---

### Step 2 — Configure CS2 Bind & Launch Options

#### A. Bind a key to send Now Playing in chat
Open the CS2 developer console (`~`) and run:

```cfg
bind "MOUSE4" "exec nowplaying.cfg"
```
*(You can also add this line to `<Steam>\steamapps\common\Counter-Strike Global Offensive\game\csgo\cfg\autoexec.cfg` so it persists permanently.)*

#### B. Enable `console.log` for Chat Triggers (`!play <song>`)
If you want CS2 chat commands like `!play <song>` to trigger playback:
1. In Steam, right-click **Counter-Strike 2** → **Properties** → **General**.
2. Under **Launch Options**, add:
   ```text
   -condebug -noclearlog
   ```

---

### Step 3 — Start the Server

Run the compiled executable:

```powershell
.\cs2nowplaying.exe
```

*(Or run from source with `go run .`)*

On launch, the server will:
1. Auto-detect your CS2 `cfg` folder across your Steam installation and secondary Steam library drives.
2. Start watching CS2's `console.log` for chat triggers (with `!play` created by default).
3. Start the local HTTP server on `http://localhost:9999`.
4. **Automatically open `http://localhost:9999` in your default browser.** *(Pass `-no-browser` if you want to suppress auto-opening the browser, or `-port 9999` to customize the port.)*

---

### Step 4 — Using YouTube `!play <song>` & Now Playing

1. Open `https://www.youtube.com` in your browser and **click anywhere on the page once** (or start any video once) so the browser unlocks background audio autoplay for that tab.
2. In CS2, when you or a teammate types in **All Chat** or **Team Chat**:
   ```text
   !play Darude Sandstorm
   ```
   The Go server resolves the first matching YouTube video ID (no YouTube API key or login required) and your open YouTube tab automatically jumps to and plays the track.
3. Press **MOUSE4** in CS2 at any time to send the current song title, artist/channel, and timestamp to CS2 chat.

---

## Web Panel Features (`http://localhost:9999`)

- **Message Builder**: Drag-and-drop blocks (`TEXT`, `ARTIST`, `TITLE`, `TIME`, `LENGTH`) and customize Unicode fonts (`Bold`, `Italic`, `Script`, `Fraktur`, `Double`, `Mono`, `Wide`, `Small Caps`, `Circled`, `Strike`).
- **Presets**: Save, load, delete, export, and import message layout presets (`configs/nowplaying_presets.json`).
- **Special Characters**: Click-to-insert music notes, symbols, arrows, shapes, emoticons, and box-drawing characters.
- **Chat Triggers & Ban Words**: Add custom plain-text or regex chat triggers, strip command prefixes (e.g. `!play`), view live matched chat events, and filter out messages containing banned words.
- **Song Queue**: View, reorder, skip (`▶ Play Next`), or clear songs requested via chat triggers.

---

## Project Structure

```text
cs2nowplaying/
├── main.go                     Main Go server, HTTP API, embedded assets & YouTube search
├── watcher.go                  CS2 console.log chat watcher & trigger engine
├── go.mod                      Go module definition
├── cs2nowplaying.exe           Compiled standalone Windows executable (with embedded UI)
│
├── youtube_nowplaying.js       Userscript: YouTube Now Playing + CS2 !play <song> auto-player
├── soundcloudartistandsong.js  Userscript: SoundCloud Now Playing scraper
├── cs2_chat_triggers.js        Userscript: SoundCloud chat trigger player
│
├── templates/
│   └── index.html              Web panel HTML (embedded into Go binary at build time)
├── static/
│   ├── style.css               Web panel styles (embedded into Go binary at build time)
│   └── app.js                  Web panel frontend logic (embedded into Go binary at build time)
│
└── configs/
    ├── nowplaying_config.json    Persisted message builder blocks
    ├── nowplaying_presets.json   Saved layout presets
    ├── nowplaying_triggers.json  Saved chat triggers (defaults to !play)
    ├── nowplaying_banwords.json  Saved chat ban words
    └── nowplaying_history.txt    Timestamped history of played tracks
```

---

## Building from Source

To recompile `cs2nowplaying.exe` after editing the Go code or any file in `templates/` or `static/`:

```powershell
go build -o cs2nowplaying.exe .
```
