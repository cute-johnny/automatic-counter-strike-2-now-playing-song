"""
CS2 Now Playing – SoundCloud -> CFG Edition
============================================
REQUIREMENTS:  pip install flask
CS2 BIND:      bind "MOUSE4" "exec nowplaying.cfg"
PANEL:         http://localhost:9999

CS2 LAUNCH OPTIONS (for chat watcher):
    -condebug -noclearlog
"""

import sys, time, threading, logging, os, json, uuid
from flask import Flask, request, jsonify, render_template
from cs2_chat_watcher import CS2ChatWatcher, Trigger, find_console_log

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(name)s] %(levelname)s: %(message)s",
    datefmt="%H:%M:%S",
)
logging.getLogger("werkzeug").setLevel(logging.ERROR)

# ─────────────────────────────────────────────
#  SOURCE ENGINE COLOR CODES
#  Special bytes \x01-\x10 interpreted by the CS2 engine.
#  Only work with playerchatwheel (not with say).
# ─────────────────────────────────────────────

COLOR_CODES = {
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

# ─────────────────────────────────────────────
#  LIVE CONFIG
#  Stores an ordered list of blocks instead of
#  a fixed prefix/suffix structure.
# ─────────────────────────────────────────────

DEFAULT_BLOCKS = [
    {"id": "b1", "type": "text",   "value": "Now Playing:", "color": "gold",       "font": "normal"},
    {"id": "b2", "type": "artist", "value": "",             "color": "silver",     "font": "normal"},
    {"id": "b3", "type": "title",  "value": "",             "color": "lightgreen", "font": "normal"},
]

class LiveConfig:
    def __init__(self):
        self._lock       = threading.Lock()
        self._blocks     = [dict(b) for b in DEFAULT_BLOCKS]
        self._use_colors = False

    def to_dict(self):
        with self._lock:
            return {
                "blocks":     [dict(b) for b in self._blocks],
                "use_colors": self._use_colors,
            }

    def update(self, blocks=None, use_colors=None):
        with self._lock:
            if blocks is not None:
                valid = []
                for b in blocks:
                    btype = b.get("type", "")
                    if btype not in ("text", "artist", "title", "time", "duration"):
                        continue
                    valid.append({
                        "id":    b.get("id", str(uuid.uuid4())[:8]),
                        "type":  btype,
                        "value": str(b.get("value", "")),
                        "color": b.get("color", "none") if b.get("color") in COLOR_CODES else "none",
                        "font":  b.get("font", "normal") if b.get("font") in UNICODE_FONTS else "normal",
                    })
                self._blocks = valid
            if use_colors is not None:
                self._use_colors = bool(use_colors)

    @property
    def has_time_blocks(self) -> bool:
        """True if any block requires playback time — avoids full to_dict() on the hot /time path."""
        with self._lock:
            return any(b["type"] in ("time", "duration") for b in self._blocks)

    def format(self, title, artist="", current_time="", duration=""):
        with self._lock:
            parts = []
            for block in self._blocks:
                btype  = block.get("type", "text")
                bcolor = COLOR_CODES.get(block.get("color", ""), "") if self._use_colors else ""
                bfont  = block.get("font", "normal")

                if btype == "text":
                    val = block.get("value", "").strip()
                    if val:
                        parts.append(f"{bcolor}{apply_font(val, bfont)}")
                elif btype == "artist":
                    if artist:
                        parts.append(f"{bcolor}{apply_font(artist, bfont)}")
                elif btype == "title":
                    parts.append(f"{bcolor}{apply_font(title, bfont)}")
                elif btype == "time":
                    if current_time:
                        parts.append(f"{bcolor}{apply_font(current_time, bfont)}")
                elif btype == "duration":
                    if duration:
                        parts.append(f"{bcolor}{apply_font(duration, bfont)}")

            return " ".join(parts)

    def get_command(self, title, artist="", current_time="", duration=""):
        message = self.format(title, artist, current_time, duration)
        safe = (message
                .replace('"', "'")
                .replace(";", ",")
                .replace("\n", " ")
                .replace("\\", ""))
        cmd = "playerchatwheel CW.Agree" if self._use_colors else "say"
        return f'{cmd} "{safe}"'


# ─────────────────────────────────────────────
#  UNICODE FONT CONVERTER
#  CS2 has no font support, but Unicode contains
#  alternate character sets that look like different
#  typefaces — no external library needed.
# ─────────────────────────────────────────────

def _make_table(upper_start, lower_start, digit_start=None,
                upper_exceptions=None, lower_exceptions=None):
    """Build a translation table for str.translate()."""
    t = {}
    ue = upper_exceptions or {}
    le = lower_exceptions or {}
    for i in range(26):
        c = chr(ord('A') + i)
        t[ord(c)] = ue.get(c, chr(upper_start + i))
        c = chr(ord('a') + i)
        t[ord(c)] = le.get(c, chr(lower_start + i))
    if digit_start is not None:
        for i in range(10):
            t[ord(str(i))] = chr(digit_start + i)
    return t

# Some Unicode math blocks have gaps — these fill them in
_ITALIC_UPPER_EX = {'H': 'ℋ'}
_ITALIC_LOWER_EX = {'h': 'ℎ', 'e': 'ℯ', 'g': 'ℊ', 'o': 'ℴ'}
_SCRIPT_UPPER_EX = {'B':'ℬ','E':'ℰ','F':'ℱ','H':'ℋ','I':'ℐ','L':'ℒ','M':'ℳ','R':'ℛ'}
_SCRIPT_LOWER_EX = {'e': 'ℯ', 'g': 'ℊ', 'o': 'ℴ'}
_FRAKTUR_UPPER_EX = {'C':'ℭ','H':'ℌ','I':'ℑ','R':'ℜ','Z':'ℨ'}
_DSTRUCK_UPPER_EX = {'C':'ℂ','H':'ℍ','N':'ℕ','P':'ℙ','Q':'ℚ','R':'ℝ','Z':'ℤ'}

UNICODE_FONTS = {
    "normal":      None,  # no conversion
    "bold":        _make_table(0x1D400, 0x1D41A, 0x1D7CE),
    "italic":      _make_table(0x1D434, 0x1D44E, None, _ITALIC_UPPER_EX, _ITALIC_LOWER_EX),
    "bold_italic": _make_table(0x1D468, 0x1D482, 0x1D7CE),
    "script":      _make_table(0x1D49C, 0x1D4B6, None, _SCRIPT_UPPER_EX, _SCRIPT_LOWER_EX),
    "bold_script": _make_table(0x1D4D0, 0x1D4EA, None),
    "fraktur":     _make_table(0x1D504, 0x1D51E, None, _FRAKTUR_UPPER_EX),
    "double":      _make_table(0x1D538, 0x1D552, 0x1D7D8, _DSTRUCK_UPPER_EX),
    "mono":        _make_table(0x1D670, 0x1D68A, 0x1D7F6),
    "wide":        _make_table(0xFF21, 0xFF41, 0xFF10),
    "small_caps":  {ord(chr(ord('a') + i)): chr(0x1D00 + i) for i in range(26)},
    "circled": {
        **{ord(chr(ord('A') + i)): chr(0x24B6 + i) for i in range(26)},
        **{ord(chr(ord('a') + i)): chr(0x24D0 + i) for i in range(26)},
        **{ord(str(i)): chr(0x2460 + i - 1) for i in range(1, 10)},
        ord('0'): '⓪',
    },
    "strikethrough": {ord(c): c + '\u0336' for c in
                      'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789'},
}

def apply_font(text: str, font: str) -> str:
    """Convert text to the chosen Unicode style."""
    table = UNICODE_FONTS.get(font)
    if not table:
        return text
    return text.translate(table)


live = LiveConfig()

# ─────────────────────────────────────────────
#  PRESETS
# ─────────────────────────────────────────────

PRESETS_FILE  = "nowplaying_presets.json"
CONFIG_FILE   = "nowplaying_config.json"    # persists live blocks/use_colors between restarts
TRIGGERS_FILE = "nowplaying_triggers.json"  # persists chat triggers between restarts

def load_presets_file():
    try:
        with open(PRESETS_FILE, "r", encoding="utf-8") as f:
            return json.load(f)
    except (IOError, json.JSONDecodeError):
        return {}

def save_presets_file(presets):
    try:
        with open(PRESETS_FILE, "w", encoding="utf-8") as f:
            json.dump(presets, f, ensure_ascii=False, indent=2)
        return True
    except IOError as e:
        log("ERROR", f"Could not save presets: {e}")
        return False

def load_config_file():
    try:
        with open(CONFIG_FILE, "r", encoding="utf-8") as f:
            return json.load(f)
    except (IOError, json.JSONDecodeError):
        return None

def save_config_file():
    try:
        with open(CONFIG_FILE, "w", encoding="utf-8") as f:
            json.dump(live.to_dict(), f, ensure_ascii=False, indent=2)
    except IOError as e:
        log("ERROR", f"Could not save config state: {e}")

def load_triggers_file() -> list[dict]:
    try:
        with open(TRIGGERS_FILE, "r", encoding="utf-8") as f:
            return json.load(f)
    except (IOError, json.JSONDecodeError):
        return []

def save_triggers_file():
    if watcher is None:
        return
    try:
        with open(TRIGGERS_FILE, "w", encoding="utf-8") as f:
            json.dump(watcher.get_triggers(), f, ensure_ascii=False, indent=2)
    except IOError as e:
        log("ERROR", f"Could not save triggers: {e}")

# ─────────────────────────────────────────────
#  STATIC CONFIG
# ─────────────────────────────────────────────

CONFIG = {
    "server_port": 9999,
    "cfg_path":    None,
    "log_file":    "nowplaying_history.txt",
}

# ─────────────────────────────────────────────
#  TERMINAL COLORS
# ─────────────────────────────────────────────

class C:
    RESET  = "\033[0m"; GREEN  = "\033[92m"; YELLOW = "\033[93m"
    RED    = "\033[91m"; CYAN   = "\033[96m"; BOLD   = "\033[1m"; DIM = "\033[2m"

def enable_ansi_windows():
    if sys.platform == "win32":
        import ctypes
        ctypes.windll.kernel32.SetConsoleMode(
            ctypes.windll.kernel32.GetStdHandle(-11), 7)

def log(level, msg):
    ts = time.strftime("%H:%M:%S")
    icons = {
        "INFO":  (C.CYAN,           "i"),
        "OK":    (C.GREEN,          "v"),
        "WARN":  (C.YELLOW,         "!"),
        "ERROR": (C.RED,            "x"),
        "MUSIC": (C.BOLD + C.GREEN, "#"),
        "FILE":  (C.CYAN,           ">"),
        "CFG":   (C.YELLOW,         "*"),
    }
    color, icon = icons.get(level, (C.RESET, "."))
    print(f"{C.DIM}[{ts}]{C.RESET} {color}[{icon}] {msg}{C.RESET}", flush=True)

# ─────────────────────────────────────────────
#  CS2 PATH DETECTION
# ─────────────────────────────────────────────

def find_cs2_cfg_path():
    rel = os.path.join("steamapps", "common",
                       "Counter-Strike Global Offensive", "game", "csgo", "cfg")
    if sys.platform == "win32":
        try:
            import winreg
            key = winreg.OpenKey(winreg.HKEY_LOCAL_MACHINE,
                                 r"SOFTWARE\WOW6432Node\Valve\Steam")
            steam, _ = winreg.QueryValueEx(key, "InstallPath")
            winreg.CloseKey(key)
            p = os.path.join(steam, rel)
            if os.path.isdir(p): return p
        except Exception:
            pass
        for root in [r"C:\Program Files (x86)\Steam", r"C:\Program Files\Steam",
                     r"D:\Steam", r"E:\Steam"]:
            p = os.path.join(root, rel)
            if os.path.isdir(p): return p
    else:
        for root in [os.path.expanduser("~/.steam/steam"),
                     os.path.expanduser("~/.local/share/Steam")]:
            p = os.path.join(root, rel)
            if os.path.isdir(p): return p
    return None

def resolve_cfg_path():
    cfg_dir = find_cs2_cfg_path()
    if cfg_dir:
        path = os.path.join(cfg_dir, "nowplaying.cfg")
        log("OK", f"Found CS2 cfg folder: {cfg_dir}")
        return path
    log("WARN", "Could not auto-detect CS2 cfg folder.")
    log("INFO", "Enter the path to the cfg folder manually:")
    print("  > ", end="", flush=True)
    user_input = input().strip().strip('"').strip("'")
    return os.path.join(user_input, "nowplaying.cfg")

# ─────────────────────────────────────────────
#  CFG FILE WRITING
# ─────────────────────────────────────────────

def write_cfg(title, artist="", current_time="", duration=""):
    cfg_path = CONFIG["cfg_path"]
    content  = live.get_command(title, artist, current_time, duration) + "\n"
    try:
        parent = os.path.dirname(cfg_path)
        if parent:
            os.makedirs(parent, exist_ok=True)
        with open(cfg_path, "w", encoding="utf-8") as f:
            f.write(content)
        log("FILE", content.strip())
        return True
    except (IOError, OSError) as e:
        log("ERROR", f"Could not write CFG file: {e}")
        return False

def write_cfg_current(placeholder=False):
    """Write CFG using the current stored song data, optionally with placeholders."""
    title    = songs.last_title    or ("Song Title"  if placeholder else "")
    artist   = songs.last_artist   or ("Artist"      if placeholder else "")
    cur_time = songs.last_time     or ("0:00"        if placeholder else "")
    duration = songs.last_duration or ("0:00"        if placeholder else "")
    if not title:
        return False
    return write_cfg(title, artist, cur_time, duration)

def log_to_history(title, artist=""):
    try:
        with open(CONFIG["log_file"], "a", encoding="utf-8") as f:
            entry = f"{artist} - {title}" if artist else title
            f.write(f"[{time.strftime('%Y-%m-%d %H:%M:%S')}] {entry}\n")
    except IOError:
        pass

# ─────────────────────────────────────────────
#  SONG HISTORY / DUPLICATE GUARD
# ─────────────────────────────────────────────

class SongHistory:
    def __init__(self):
        self._last_title    = None
        self._last_artist   = ""
        self._last_time     = ""
        self._last_duration = ""
        self._lock = threading.Lock()

    def is_new(self, title, artist=""):
        with self._lock:
            if title == self._last_title and artist == self._last_artist:
                return False
            self._last_title  = title
            self._last_artist = artist
            return True

    def update_time(self, current_time, duration=""):
        with self._lock:
            self._last_time = current_time
            if duration:
                self._last_duration = duration

    @property
    def last_title(self):    return self._last_title    or ""
    @property
    def last_artist(self):   return self._last_artist   or ""
    @property
    def last_time(self):     return self._last_time     or ""
    @property
    def last_duration(self): return self._last_duration or ""

songs = SongHistory()

# Chat watcher — started in main() once cfg_path is known
watcher: CS2ChatWatcher | None = None

# ─────────────────────────────────────────────
#  FLASK
# ─────────────────────────────────────────────

app = Flask(__name__)


@app.route("/")
def panel():
    return render_template("index.html")


# ── Config ───────────────────────────────────

@app.route("/config", methods=["GET"])
def config_get():
    d = live.to_dict()
    d["last_song"]     = songs.last_title
    d["last_artist"]   = songs.last_artist
    d["last_time"]     = songs.last_time
    d["last_duration"] = songs.last_duration
    return jsonify(d)


@app.route("/config", methods=["POST"])
def config_set():
    try:
        data = request.get_json(force=True)
    except Exception:
        return jsonify({"error": "invalid JSON"}), 400

    live.update(
        blocks     = data.get("blocks"),
        use_colors = data.get("use_colors"),
    )
    log("CFG", f"Config updated ({len(live.to_dict()['blocks'])} blocks)")

    # Always rewrite CFG after a config change.
    # Use real data if a song is playing, otherwise write placeholders.
    title    = songs.last_title    or "Song Title"
    artist   = songs.last_artist   or "Artist"
    cur_time = songs.last_time     or "0:00"
    duration = songs.last_duration or "0:00"
    write_cfg(title, artist, cur_time, duration)
    log("OK", f"CFG written ({'real data' if songs.last_title else 'placeholder — waiting for SoundCloud'}).\n")
    save_config_file()

    return jsonify({"ok": True, "used_placeholder": not bool(songs.last_title)})


# ── Time ─────────────────────────────────────

@app.route("/time", methods=["POST"])
def receive_time():
    """Receives current playback time and song duration every 3 seconds from Tampermonkey."""
    try:
        data         = request.get_json(force=True)
        current_time = data.get("current_time", "").strip()
        duration     = data.get("duration", "").strip()
    except Exception:
        return jsonify({"error": "invalid JSON"}), 400

    if not current_time:
        return jsonify({"error": "empty time"}), 400

    songs.update_time(current_time, duration)

    # Only rewrite the CFG if the layout actually uses a time or duration block.
    # Otherwise rewriting every second would be wasteful.
    if live.has_time_blocks and songs.last_title:
        write_cfg_current()

    return jsonify({"status": "ok"}), 200


# ── Song ─────────────────────────────────────

@app.route("/song", methods=["POST"])
def receive_song():
    try:
        data   = request.get_json(force=True)
        title  = data.get("title",  "").strip()
        artist = data.get("artist", "").strip()
    except Exception:
        return jsonify({"error": "invalid JSON"}), 400

    if not title:
        return jsonify({"error": "empty title"}), 400

    if not songs.is_new(title, artist):
        log("INFO", f"Duplicate — skipped: {artist!r} - {title!r}")
        return jsonify({"status": "duplicate"}), 200

    display = f"{artist} - {title}" if artist else title
    log("MUSIC", f"{C.BOLD}{display}{C.RESET}")
    log_to_history(title, artist)

    if write_cfg_current():
        log("OK", "CFG updated! Press MOUSE4/MOUSE5 in CS2 to send.\n")
        return jsonify({"status": "cfg_updated"}), 200
    else:
        return jsonify({"status": "write_error"}), 500


# ── Presets ───────────────────────────────────

@app.route("/presets", methods=["GET"])
def presets_list():
    return jsonify(load_presets_file())


@app.route("/presets", methods=["POST"])
def presets_save():
    try:
        data = request.get_json(force=True)
        name = data.get("name", "").strip()
    except Exception:
        return jsonify({"error": "invalid JSON"}), 400

    if not name:
        return jsonify({"error": "empty name"}), 400

    presets = load_presets_file()
    presets[name] = live.to_dict()
    presets[name].pop("last_song",   None)
    presets[name].pop("last_artist", None)

    if save_presets_file(presets):
        log("CFG", f"Saved preset: {name!r}")
        return jsonify({"ok": True, "name": name})
    else:
        return jsonify({"error": "write failed"}), 500


@app.route("/presets/import", methods=["POST"])
def presets_import():
    try:
        incoming = request.get_json(force=True)
        if not isinstance(incoming, dict):
            raise ValueError("expected object")
    except Exception:
        return jsonify({"error": "invalid JSON"}), 400

    presets = load_presets_file()
    imported = 0
    for name, cfg in incoming.items():
        if isinstance(cfg, dict) and "blocks" in cfg:
            presets[str(name)] = cfg
            imported += 1

    save_presets_file(presets)
    log("CFG", f"Imported {imported} preset(s)")
    return jsonify({"ok": True, "imported": imported})

@app.route("/presets/<name>", methods=["DELETE"])
def presets_delete(name):
    presets = load_presets_file()
    if name not in presets:
        return jsonify({"error": "not found"}), 404
    del presets[name]
    save_presets_file(presets)
    log("CFG", f"Deleted preset: {name!r}")
    return jsonify({"ok": True})


@app.route("/presets/<name>/load", methods=["POST"])
def presets_load(name):
    presets = load_presets_file()
    if name not in presets:
        return jsonify({"error": "not found"}), 404

    preset = presets[name]
    live.update(
        blocks     = preset.get("blocks"),
        use_colors = preset.get("use_colors"),
    )
    log("CFG", f"Loaded preset: {name!r}")
    write_cfg_current(placeholder=True)
    save_config_file()
    return jsonify({"ok": True, "config": live.to_dict()})



# ─────────────────────────────────────────────
#  CHAT WATCHER ENDPOINTS
# ─────────────────────────────────────────────

@app.route("/watcher/status", methods=["GET"])
def watcher_status():
    """Returns watcher health and log file info."""
    if watcher is None:
        return jsonify({"running": False, "reason": "not initialized"})
    return jsonify({
        "running":    watcher.is_running,
        "log_path":   watcher.log_path,
        "log_exists": watcher.log_exists,
    })


@app.route("/watcher/triggers", methods=["GET"])
def triggers_list():
    if watcher is None:
        return jsonify([])
    return jsonify(watcher.get_triggers())


@app.route("/watcher/triggers", methods=["POST"])
def triggers_add():
    if watcher is None:
        return jsonify({"error": "watcher not running"}), 503
    try:
        data = request.get_json(force=True)
    except Exception:
        return jsonify({"error": "invalid JSON"}), 400

    label        = data.get("label", "").strip()
    pattern      = data.get("pattern", "").strip()
    regex        = data.get("regex", "").strip()
    strip_prefix = data.get("strip_prefix", "").strip()

    if not label:
        return jsonify({"error": "label is required"}), 400
    if not pattern and not regex:
        return jsonify({"error": "pattern or regex is required"}), 400

    trigger = Trigger(
        id           = str(uuid.uuid4())[:8],
        label        = label,
        pattern      = pattern,
        regex        = regex,
        enabled      = data.get("enabled", True),
        strip_prefix = strip_prefix,
    )
    watcher.add_trigger(trigger)
    save_triggers_file()
    return jsonify({"ok": True, "trigger": trigger.to_dict()})


@app.route("/watcher/triggers/<trigger_id>", methods=["DELETE"])
def triggers_delete(trigger_id):
    if watcher is None:
        return jsonify({"error": "watcher not running"}), 503
    existed = watcher.remove_trigger(trigger_id)
    if not existed:
        return jsonify({"error": "not found"}), 404
    save_triggers_file()
    return jsonify({"ok": True, "triggers": watcher.get_triggers()})


@app.route("/watcher/triggers/<trigger_id>", methods=["PATCH"])
def triggers_update(trigger_id):
    if watcher is None:
        return jsonify({"error": "watcher not running"}), 503
    try:
        data = request.get_json(force=True)
    except Exception:
        return jsonify({"error": "invalid JSON"}), 400

    t = watcher.update_trigger(trigger_id, **{
        k: v for k, v in data.items()
        if k in ("label", "pattern", "regex", "enabled", "strip_prefix")
    })
    if t is None:
        return jsonify({"error": "not found"}), 404
    save_triggers_file()
    return jsonify({"ok": True, "trigger": t.to_dict(), "triggers": watcher.get_triggers()})


@app.route("/watcher/events", methods=["GET"])
def watcher_events():
    """
    Poll endpoint for the browser script.
    Returns the most recent matched events.
    Optional query param: ?limit=N (default 50)
    """
    if watcher is None:
        return jsonify([])
    limit = min(int(request.args.get("limit", 50)), 200)
    return jsonify(watcher.get_recent_events(limit))


@app.route("/watcher/banwords", methods=["GET"])
def banwords_list():
    if watcher is None:
        return jsonify([])
    return jsonify(watcher.get_ban_words())


@app.route("/watcher/banwords", methods=["POST"])
def banwords_add():
    if watcher is None:
        return jsonify({"error": "watcher not running"}), 503
    try:
        data = request.get_json(force=True)
        word = data.get("word", "").strip()
    except Exception:
        return jsonify({"error": "invalid JSON"}), 400

    if not word:
        return jsonify({"error": "empty word"}), 400

    added = watcher.add_ban_word(word)
    return jsonify({"ok": True, "word": word.lower(), "added": added})


@app.route("/watcher/banwords/<word>", methods=["DELETE"])
def banwords_delete(word):
    if watcher is None:
        return jsonify({"error": "watcher not running"}), 503
    existed = watcher.remove_ban_word(word)
    if not existed:
        return jsonify({"error": "not found"}), 404
    return jsonify({"ok": True})


# ─────────────────────────────────────────────
#  MAIN
# ─────────────────────────────────────────────

def main():
    enable_ansi_windows()
    print(f"""
{C.CYAN}{C.BOLD}+------------------------------------------------------+
|    CS2 Now Playing  .  SoundCloud -> CFG Edition     |
+------------------------------------------------------+{C.RESET}
""")

    CONFIG["cfg_path"] = resolve_cfg_path()
    print()

    # ── Start chat watcher ────────────────────
    global watcher
    console_log_path = find_console_log(CONFIG["cfg_path"])
    watcher = CS2ChatWatcher(console_log_path)
    watcher.start()

    # Restore triggers from last session
    saved_triggers = load_triggers_file()
    for t_data in saved_triggers:
        try:
            watcher.add_trigger(Trigger(**t_data))
        except Exception as e:
            log("WARN", f"Skipped invalid saved trigger: {e}")
    if saved_triggers:
        log("OK", f"Restored {len(saved_triggers)} trigger(s) from {TRIGGERS_FILE}")

    # Restore live config from last session (blocks + use_colors)
    saved_config = load_config_file()
    if saved_config:
        live.update(
            blocks     = saved_config.get("blocks"),
            use_colors = saved_config.get("use_colors"),
        )
        log("OK", f"Restored config state from {CONFIG_FILE}")

    log("INFO", f"Server / Panel : http://localhost:{CONFIG['server_port']}")
    log("INFO", f"CFG file       : {CONFIG['cfg_path']}")
    log("INFO", f"Console log    : {console_log_path}")
    log("INFO", f"History file   : {CONFIG['log_file']}")
    log("INFO", f"Presets file   : {PRESETS_FILE}")
    print()
    log("WARN", 'CS2 bind       : bind "MOUSE4" "exec nowplaying.cfg"')
    log("WARN", 'CS2 launch opt : -condebug -noclearlog  (required for chat watcher)')
    print()
    log("INFO", "Waiting for data from SoundCloud...\n")

    app.run(
        host="127.0.0.1",
        port=CONFIG["server_port"],
        debug=False,
        use_reloader=False,
    )


if __name__ == "__main__":
    main()