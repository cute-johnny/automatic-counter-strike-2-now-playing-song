"""
cs2_chat_watcher.py
====================
Watches the CS2 console.log file (enabled via -condebug -noclearlog launch options)
and fires callbacks when chat messages match user-defined triggers.

REQUIRED CS2 LAUNCH OPTIONS:
    -condebug -noclearlog

LOG FILE LOCATION (auto-detected):
    <CS2 install>/game/csgo/console.log

HOW IT WORKS:
    1. On start, seeks to the END of the log file (ignores history)
    2. Polls for new lines every POLL_INTERVAL seconds
    3. Only processes lines that contain [ALL] (global chat)
    4. Checks each matching line against registered triggers
    5. Fires registered callbacks with the event data

EXTENSIBILITY:
    Triggers support both plain string matching and regex.
    The callback receives a rich ChatEvent dict so future
    actions (e.g. respond in chat, trigger hotkeys, update overlay)
    can be added without changing this module.
"""

import os
import re
import time
import uuid
import threading
import logging
from dataclasses import dataclass, field, asdict
from typing import Callable, Optional
from collections import deque

log = logging.getLogger("cs2_watcher")

# ─────────────────────────────────────────────
#  CONSTANTS
# ─────────────────────────────────────────────

POLL_INTERVAL   = 0.5   # seconds between log file checks
REQUIRED_TOKEN  = "[ALL]"  # only lines containing this are processed
MAX_EVENT_QUEUE = 200   # max recent events kept in memory

# Pattern to parse CS2 chat lines:
# "03/22 02:18:48  [ALL] PlayerName [DEAD]: message"
_CHAT_RE = re.compile(
    r"(?P<timestamp>\d{2}/\d{2}\s+\d{2}:\d{2}:\d{2})\s+"
    r"\[ALL\]\s+(?P<player>.+?)\s*:\s*(?P<message>.+)"
)

# Strip game-status tags that CS2 appends to the player name in the log
# e.g. "PlayerName [DEAD]" -> "PlayerName"
_PLAYER_TAG_RE = re.compile(r"\s*\[(?:DEAD|SPEC|BOT)\]\s*$", re.IGNORECASE)

# ─────────────────────────────────────────────
#  DATA STRUCTURES
# ─────────────────────────────────────────────

@dataclass
class Trigger:
    """A single user-defined trigger rule."""
    id:           str
    label:        str
    pattern:      str        # plain-text substring to match (case-insensitive)
    regex:        str = ""   # optional regex (takes priority if set)
    enabled:      bool = True
    strip_prefix: str = ""   # if set, everything AFTER this prefix becomes search_query
    # future fields: action, response_template, cooldown_sec, etc.

    def matches(self, message: str) -> bool:
        """Returns True if this trigger fires on the given chat message."""
        if not self.enabled:
            return False
        if self.regex:
            try:
                return bool(re.search(self.regex, message, re.IGNORECASE))
            except re.error:
                return False
        return self.pattern.lower() in message.lower()

    def extract_query(self, message: str) -> str:
        """
        Return the search query from the message.
        If strip_prefix is set, returns everything after the first occurrence
        of that prefix (case-insensitive), stripped.
        Falls back to the full message if prefix is not found.
        """
        if not self.strip_prefix:
            return message.strip()
        lower_msg    = message.lower()
        lower_prefix = self.strip_prefix.lower()
        idx = lower_msg.find(lower_prefix)
        if idx == -1:
            return message.strip()
        after = message[idx + len(self.strip_prefix):]
        return after.strip()

    def to_dict(self) -> dict:
        return asdict(self)


@dataclass
class ChatEvent:
    """Fired when a chat line matches a trigger."""
    id:           str
    timestamp:    str        # raw timestamp from log
    player:       str        # player name
    message:      str        # full raw chat message text
    raw_line:     str        # full original log line
    trigger_id:    str        # which trigger fired
    trigger_label: str        # human-readable trigger label
    strip_prefix:  str = ""   # the prefix that was stripped to produce search_query
    search_query:  str = ""   # extracted query (after stripping prefix, if any)
    received_at:  float = field(default_factory=time.time)

    def to_dict(self) -> dict:
        return asdict(self)


# ─────────────────────────────────────────────
#  WATCHER
# ─────────────────────────────────────────────

class CS2ChatWatcher:
    """
    Monitors CS2's console.log for new [ALL] chat lines and
    dispatches ChatEvent objects to registered callbacks.

    Usage:
        watcher = CS2ChatWatcher(log_path)
        watcher.add_callback(my_function)
        watcher.add_trigger(Trigger(id="t1", label="Hello", pattern="hello"))
        watcher.start()
    """

    def __init__(self, log_path: str):
        self.log_path   = log_path
        self._position  = 0           # byte offset into the log file
        self._triggers: dict[str, Trigger] = {}
        self._callbacks: list[Callable]    = []
        self._events: deque[ChatEvent]     = deque(maxlen=MAX_EVENT_QUEUE)
        self._ban_words: set[str]           = set()   # case-insensitive
        self._lock    = threading.Lock()
        self._running = False
        self._thread: Optional[threading.Thread] = None

    # ── Public API ───────────────────────────

    def start(self):
        """Start the background watcher thread."""
        if self._running:
            return
        self._running = True
        self._seek_to_end()
        self._thread = threading.Thread(
            target=self._watch_loop,
            name="CS2ChatWatcher",
            daemon=True,
        )
        self._thread.start()
        log.info(f"CS2ChatWatcher started — watching: {self.log_path}")

    def stop(self):
        """Stop the background watcher thread."""
        self._running = False
        if self._thread:
            self._thread.join(timeout=3)
        log.info("CS2ChatWatcher stopped.")

    def add_callback(self, fn: Callable[[ChatEvent], None]):
        """Register a function to call on every matched event."""
        with self._lock:
            self._callbacks.append(fn)

    def add_trigger(self, trigger: Trigger) -> Trigger:
        """Register a new trigger rule."""
        with self._lock:
            self._triggers[trigger.id] = trigger
        log.info(f"Trigger added: {trigger.label!r} (id={trigger.id})")
        return trigger

    def update_trigger(self, trigger_id: str, **kwargs) -> Optional[Trigger]:
        """Update fields on an existing trigger."""
        with self._lock:
            t = self._triggers.get(trigger_id)
            if not t:
                return None
            for k, v in kwargs.items():
                if hasattr(t, k):
                    setattr(t, k, v)
        return t

    def remove_trigger(self, trigger_id: str) -> bool:
        """Remove a trigger by id. Returns True if it existed."""
        with self._lock:
            existed = trigger_id in self._triggers
            self._triggers.pop(trigger_id, None)
        return existed

    def get_triggers(self) -> list[dict]:
        with self._lock:
            return [t.to_dict() for t in self._triggers.values()]

    def get_recent_events(self, limit: int = 50) -> list[dict]:
        with self._lock:
            events = list(self._events)
        return [e.to_dict() for e in reversed(events)][:limit]

    def add_ban_word(self, word: str) -> bool:
        """Add a word to the ban list. Returns True if it was new."""
        w = word.lower().strip()
        if not w:
            return False
        with self._lock:
            added = w not in self._ban_words
            self._ban_words.add(w)
        if added:
            log.info(f"Ban word added: {w!r}")
        return added

    def remove_ban_word(self, word: str) -> bool:
        """Remove a word from the ban list. Returns True if it existed."""
        w = word.lower().strip()
        with self._lock:
            existed = w in self._ban_words
            self._ban_words.discard(w)
        if existed:
            log.info(f"Ban word removed: {w!r}")
        return existed

    def get_ban_words(self) -> list[str]:
        with self._lock:
            return sorted(self._ban_words)

    def _is_banned(self, message: str) -> bool:
        """Returns True if the message contains any ban word."""
        msg_lower = message.lower()
        with self._lock:
            return any(bw in msg_lower for bw in self._ban_words)

    @property
    def is_running(self) -> bool:
        return self._running and (self._thread is not None) and self._thread.is_alive()

    @property
    def log_exists(self) -> bool:
        return os.path.isfile(self.log_path)

    # ── Internal ─────────────────────────────

    def _seek_to_end(self):
        """Skip all existing log content — only watch NEW lines."""
        try:
            if os.path.isfile(self.log_path):
                self._position = os.path.getsize(self.log_path)
                log.debug(f"Seeked to end of log: byte {self._position}")
            else:
                self._position = 0
                log.warning(f"Log file not found yet: {self.log_path}")
        except OSError as e:
            log.warning(f"Could not seek log file: {e}")
            self._position = 0

    def _watch_loop(self):
        while self._running:
            try:
                self._read_new_lines()
            except Exception as e:
                log.error(f"Watcher error: {e}")
            time.sleep(POLL_INTERVAL)

    def _read_new_lines(self):
        """Read any lines added since the last check."""
        if not os.path.isfile(self.log_path):
            return  # file doesn't exist yet — CS2 not started

        file_size = os.path.getsize(self.log_path)

        # File was truncated (e.g. game restarted without -noclearlog)
        if file_size < self._position:
            log.warning("Log file shrank — resetting position to 0.")
            self._position = 0

        # Nothing new
        if file_size == self._position:
            return

        # Read only the new bytes
        # Open in BINARY mode so f.tell() returns real byte offsets.
        # Text mode on Windows translates \r\n -> \n, making tell() lie
        # about the position and causing the same lines to be re-read.
        with open(self.log_path, "rb") as f:
            f.seek(self._position)
            raw = f.read()
            self._position = f.tell()

        new_content = raw.decode("utf-8", errors="replace").replace("\r\n", "\n").replace("\r", "\n")

        for line in new_content.splitlines():
            self._process_line(line)

    def _process_line(self, line: str):
        """Filter, parse, and dispatch a single log line."""
        # REQUIRED: line must contain [ALL]
        if REQUIRED_TOKEN not in line:
            return

        parsed = _CHAT_RE.match(line)
        if not parsed:
            # Malformed [ALL] line — still check triggers against the raw line
            player    = ""
            message   = line
            timestamp = ""
        else:
            timestamp = parsed.group("timestamp")
            player    = _PLAYER_TAG_RE.sub("", parsed.group("player")).strip()
            message   = parsed.group("message").strip()

        log.debug(f"[ALL] {player}: {message}")

        # Check ban words first — if any ban word is in the message, drop it
        if self._is_banned(message):
            log.debug(f"Banned message dropped: {message!r}")
            return

        # Check against all triggers
        with self._lock:
            triggers_snapshot = list(self._triggers.values())

        for trigger in triggers_snapshot:
            if trigger.matches(message):
                event = ChatEvent(
                    id            = str(uuid.uuid4())[:8],
                    timestamp     = timestamp,
                    player        = player,
                    message       = message,
                    raw_line      = line,
                    trigger_id    = trigger.id,
                    trigger_label = trigger.label,
                    strip_prefix  = trigger.strip_prefix,
                    search_query  = trigger.extract_query(message),
                )
                self._dispatch(event)

    def _dispatch(self, event: ChatEvent):
        with self._lock:
            self._events.append(event)
            callbacks = list(self._callbacks)

        log.info(
            f"MATCH [{event.trigger_label}] "
            f"{event.player}: {event.message}"
        )

        for fn in callbacks:
            try:
                fn(event)
            except Exception as e:
                log.error(f"Callback error: {e}")


# ─────────────────────────────────────────────
#  HELPERS
# ─────────────────────────────────────────────

def find_console_log(cs2_cfg_path: str) -> str:
    """
    Derive the console.log path from either:
      - the cfg FILE  path: .../game/csgo/cfg/nowplaying.cfg
      - the cfg FOLDER path: .../game/csgo/cfg
    console.log lives at:   .../game/csgo/console.log
    """
    p = cs2_cfg_path
    # If it's a file, step up to the cfg directory first
    if os.path.isfile(p) or p.endswith(".cfg"):
        p = os.path.dirname(p)   # .../csgo/cfg
    # Now step up one more level: cfg -> csgo
    csgo_dir = os.path.dirname(p)
    return os.path.join(csgo_dir, "console.log")