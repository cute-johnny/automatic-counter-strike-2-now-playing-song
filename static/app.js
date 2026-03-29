/* ═══════════════════════════════════════════════════════
   CS2 Now Playing – app.js
   ═══════════════════════════════════════════════════════ */

/* ── UNICODE FONT CONVERTER ───────────────────────────── */

function makeTable(upperStart, lowerStart, digitStart, upperEx, lowerEx) {
  const t = {};
  for (let i = 0; i < 26; i++) {
    const U = String.fromCharCode(65 + i);
    const l = String.fromCharCode(97 + i);
    t[U] = (upperEx && upperEx[U]) || String.fromCodePoint(upperStart + i);
    t[l] = (lowerEx && lowerEx[l]) || String.fromCodePoint(lowerStart + i);
  }
  if (digitStart != null) {
    for (let i = 0; i < 10; i++)
      t[String(i)] = String.fromCodePoint(digitStart + i);
  }
  return t;
}

const ITALIC_UEX = { H: 'ℋ' };
const ITALIC_LEX = { h: 'ℎ', e: 'ℯ', g: 'ℊ', o: 'ℴ' };
const SCRIPT_UEX = { B:'ℬ',E:'ℰ',F:'ℱ',H:'ℋ',I:'ℐ',L:'ℒ',M:'ℳ',R:'ℛ' };
const SCRIPT_LEX = { e:'ℯ', g:'ℊ', o:'ℴ' };
const FRAK_UEX   = { C:'ℭ',H:'ℌ',I:'ℑ',R:'ℜ',Z:'ℨ' };
const DSTR_UEX   = { C:'ℂ',H:'ℍ',N:'ℕ',P:'ℙ',Q:'ℚ',R:'ℝ',Z:'ℤ' };

const FONT_TABLES = {
  normal:        null,
  bold:          makeTable(0x1D400, 0x1D41A, 0x1D7CE),
  italic:        makeTable(0x1D434, 0x1D44E, null,    ITALIC_UEX, ITALIC_LEX),
  bold_italic:   makeTable(0x1D468, 0x1D482, 0x1D7CE),
  script:        makeTable(0x1D49C, 0x1D4B6, null,    SCRIPT_UEX, SCRIPT_LEX),
  bold_script:   makeTable(0x1D4D0, 0x1D4EA, null),
  fraktur:       makeTable(0x1D504, 0x1D51E, null,    FRAK_UEX),
  double:        makeTable(0x1D538, 0x1D552, 0x1D7D8, DSTR_UEX),
  mono:          makeTable(0x1D670, 0x1D68A, 0x1D7F6),
  wide:          makeTable(0xFF21,  0xFF41,  0xFF10),
  small_caps: (() => {
    const t = {};
    const sc = 'ᴀʙᴄᴅᴇꜰɢʜɪᴊᴋʟᴍɴᴏᴘǫʀsᴛᴜᴠᴡxʏᴢ';
    for (let i = 0; i < 26; i++) t[String.fromCharCode(97 + i)] = sc[i];
    return t;
  })(),
  circled: (() => {
    const t = {};
    for (let i = 0; i < 26; i++) t[String.fromCharCode(65 + i)] = String.fromCodePoint(0x24B6 + i);
    for (let i = 0; i < 26; i++) t[String.fromCharCode(97 + i)] = String.fromCodePoint(0x24D0 + i);
    for (let i = 1; i <= 9; i++) t[String(i)] = String.fromCodePoint(0x2460 + i - 1);
    t['0'] = '⓪';
    return t;
  })(),
  strikethrough: (() => {
    const t = {};
    for (const c of 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789')
      t[c] = c + '\u0336';
    return t;
  })(),
};

const FONT_LABELS = {
  normal:        'Normal',
  bold:          '𝐁𝐨𝐥𝐝',
  italic:        '𝐼𝑡𝑎𝑙𝑖𝑐',
  bold_italic:   '𝑩𝒐𝒍𝒅 𝑰𝒕𝒂𝒍𝒊𝒄',
  script:        '𝒮𝒸𝓇𝒾𝓅𝓉',
  bold_script:   '𝓑𝓸𝓵𝓭 𝓢𝓬𝓻𝓲𝓹𝓽',
  fraktur:       '𝔉𝔯𝔞𝔨𝔱𝔲𝔯',
  double:        '𝔻𝕠𝕦𝕓𝕝𝕖',
  mono:          '𝙼𝚘𝚗𝚘',
  wide:          'Ｗｉｄｅ',
  small_caps:    'Sᴍᴀʟʟ Cᴀᴘs',
  circled:       'Ⓒⓘⓡⓒⓛⓔⓓ',
  strikethrough: 'S̶t̶r̶i̶k̶e̶',
};

function applyFont(text, fontKey) {
  const table = FONT_TABLES[fontKey];
  if (!table) return text;
  return [...text].map(ch => table[ch] ?? ch).join('');
}

/* ── CONSTANTS ────────────────────────────────────────── */

const COLORS = {
  none:       { label: "None",       hex: "#777777" },
  default:    { label: "White",      hex: "#FFFFFF" },
  team:       { label: "Team",       hex: "#FF8C00" },
  lightgreen: { label: "Lime",       hex: "#BEFF00" },
  green:      { label: "Green",      hex: "#3FBF3F" },
  olive:      { label: "Olive",      hex: "#9AAA72" },
  gold:       { label: "Gold",       hex: "#FFD700" },
  silver:     { label: "Silver",     hex: "#C0C0C0" },
  red:        { label: "Red",        hex: "#FF4040" },
  pink:       { label: "Pink",       hex: "#FF69B4" },
  lightred:   { label: "Light red",  hex: "#FF8080" },
  purple:     { label: "Purple",     hex: "#CC66FF" },
  lightblue:  { label: "Light blue", hex: "#6BD4F5" },
};

const EXAMPLE = {
  title:    "Ben 10 (2005) Intro PL",
  artist:   "Cartoon Network",
  time:     "1:55",
  duration: "3:18",
};

const CHAR_CATS = {
  "chars-music":   ["♩","♪","♫","♬","♭","♮","♯","🎵","🎶","🎸","🎹","🎺","🎻","🥁","🎤","🎧","🎼"],
  "chars-symbols": ["★","☆","✦","✧","✩","✪","✫","✬","✭","✮","✯","✰","❤","♥","♦","♠","♣","♡","♢","♤","♧","✓","✔","✗","✘","✚","✞","✠","✡","⚡","⚠","⚔","⚙","🔥","💎","👑","🏆","🎯","💀"],
  "chars-arrows":  ["→","←","↑","↓","↔","↕","↗","↙","↖","↘","➜","➡","⬅","⬆","⬇","➤","►","◄","▶","◀","⇒","⇐","⇑","⇓","⇔","⇕","↩","↪"],
  "chars-shapes":  ["■","□","▪","▫","▬","◼","◻","●","○","◉","◎","◕","◔","▲","▼","△","▽","◆","◇","◈","⬛","⬜","🔶","🔷","🔸","🔹","🔺","🔻"],
  "chars-emotes":  ["☺","☻","ツ","シ","ヅ","ʕ•ᴥ•ʔ","⌐▨_▨","◕‿◕","^_^","o_O","¯\\_(ツ)_/¯","(╯°□°）╯","( ͡° ͜ʖ ͡°)","ʕ ᵔᴥᵔ ʔ"],
  "chars-lines":   ["─","━","│","┃","┄","┅","┆","┇","╔","╗","╚","╝","║","═","╠","╣","╦","╩","╬","┌","┐","└","┘","├","┤","┬","┴","┼","▔","▕","▏","░","▒","▓"],
  "chars-misc":    ["©","®","™","§","¶","№","°","±","×","÷","∞","≈","≠","≤","≥","①","②","③","④","⑤","⑥","⑦","⑧","⑨","⑩","«","»","‹","›","„","\u201C","\u2018","\u2019","…","·","•","◦"],
};

/* ── STATE ────────────────────────────────────────────── */

let blocks      = [];
let useColors   = false;
let activeInput = null;   // last focused TEXT input (for char picker)
let activePreset = null;  // currently loaded preset name
let dragSrcIdx  = null;   // block index being dragged

let _idCounter = 0;
function newId() { return "b" + (++_idCounter); }

/* ── BLOCK HELPERS ────────────────────────────────────── */

function addBlock(type) {
  const defaults = {
    text:     { value: "", color: "gold",       font: "normal" },
    artist:   { value: "", color: "silver",     font: "normal" },
    title:    { value: "", color: "lightgreen", font: "normal" },
    time:     { value: "", color: "silver",     font: "normal" },
    duration: { value: "", color: "olive",      font: "normal" },
  };
  const d = defaults[type] || defaults.text;
  blocks.push({ id: newId(), type, value: d.value, color: d.color, font: d.font });
  renderBlocks();
  updatePreview();
  scheduleApply();
}

function removeBlock(idx) {
  blocks.splice(idx, 1);
  renderBlocks();
  updatePreview();
  scheduleApply();
}

/* ── RENDER BLOCKS ────────────────────────────────────── */

function renderBlocks() {
  const list = document.getElementById("block-list");
  list.innerHTML = "";
  if (blocks.length === 0) {
    list.appendChild(makeEmptyMsg());
    return;
  }
  blocks.forEach((block, i) => list.appendChild(makeBlockEl(block, i)));
}

function makeEmptyMsg() {
  const d = document.createElement("div");
  d.className = "block-empty";
  d.id = "block-empty";
  d.textContent = "No blocks — add some from the palette above";
  return d;
}

function makeBlockEl(block, idx) {
  const item = document.createElement("div");
  item.className = "block-item";
  item.draggable = false; // only enabled when grabbing the handle

  /* ── Drag events ── */
  item.addEventListener("dragstart", (e) => {
    if (!item.draggable) { e.preventDefault(); return; }
    dragSrcIdx = idx;
    item.classList.add("dragging");
    e.dataTransfer.effectAllowed = "move";
    e.dataTransfer.setData("text/plain", String(idx));
  });

  item.addEventListener("dragend", () => {
    item.draggable = false;
    item.classList.remove("dragging");
    document.querySelectorAll(".block-item").forEach(el => el.classList.remove("drag-over"));
    document.getElementById("block-list").classList.remove("drag-over-zone");
  });

  item.addEventListener("dragover", (e) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    document.querySelectorAll(".block-item").forEach(el => el.classList.remove("drag-over"));
    item.classList.add("drag-over");
  });

  item.addEventListener("dragleave", () => item.classList.remove("drag-over"));

  item.addEventListener("drop", (e) => {
    e.preventDefault();
    item.classList.remove("drag-over");
    if (dragSrcIdx === null || dragSrcIdx === idx) return;
    const moved = blocks.splice(dragSrcIdx, 1)[0];
    blocks.splice(idx, 0, moved);
    dragSrcIdx = null;
    renderBlocks();
    updatePreview();
    scheduleApply();
  });

  /* ── Drag handle ── */
  const handle = document.createElement("span");
  handle.className = "drag-handle";
  handle.textContent = "⠿";
  handle.addEventListener("mousedown", () => { item.draggable = true; });
  handle.addEventListener("mouseleave", () => {
    if (!item.classList.contains("dragging")) item.draggable = false;
  });

  /* ── Badge ── */
  const badge = document.createElement("span");
  badge.className = `block-badge block-badge-${block.type}`;
  badge.textContent = {
    text: "TEXT", artist: "ARTIST", title: "TITLE",
    time: "TIME", duration: "LENGTH",
  }[block.type] ?? block.type.toUpperCase();

  /* ── Content (input for TEXT, label for others) ── */
  let contentEl;
  if (block.type === "text") {
    contentEl = document.createElement("input");
    contentEl.type = "text";
    contentEl.className = "block-input";
    contentEl.value = block.value;
    contentEl.placeholder = "type text...";

    contentEl.addEventListener("input", () => {
      blocks[idx].value = contentEl.value;
      updatePreview();
      scheduleApply();
    });

    contentEl.addEventListener("focus", () => {
      activeInput = contentEl;
      const lbl = document.getElementById("active-field-label");
      lbl.textContent = `TEXT block #${idx + 1}`;
      lbl.style.color = "#ff6b35";
    });
  } else {
    contentEl = document.createElement("span");
    contentEl.className = "block-label";
    contentEl.textContent = {
      artist:   "← artist name from SoundCloud",
      title:    "← song title from SoundCloud",
      time:     "← current playback time (e.g. 1:55)",
      duration: "← song length (e.g. 3:18)",
    }[block.type] ?? "";
  }

  /* ── Color select ── */
  const colorSel = document.createElement("select");
  colorSel.className = "block-color-select";

  for (const [key, meta] of Object.entries(COLORS)) {
    const opt = document.createElement("option");
    opt.value = key;
    opt.textContent = meta.label;
    if (key === block.color) opt.selected = true;
    colorSel.appendChild(opt);
  }

  function applySelectColor() {
    const hex = COLORS[colorSel.value]?.hex ?? "#ccc";
    colorSel.style.color = colorSel.value === "none" ? "#555" : hex;
  }
  applySelectColor();

  colorSel.addEventListener("change", () => {
    blocks[idx].color = colorSel.value;
    applySelectColor();
    updatePreview();
    scheduleApply();
  });

  colorSel.style.display = useColors ? "" : "none";

  /* ── Font select ── */
  const fontSel = document.createElement("select");
  fontSel.className = "block-font-select";

  for (const [key, label] of Object.entries(FONT_LABELS)) {
    const opt = document.createElement("option");
    opt.value = key;
    opt.textContent = label;
    if (key === (block.font || "normal")) opt.selected = true;
    fontSel.appendChild(opt);
  }

  fontSel.addEventListener("change", () => {
    blocks[idx].font = fontSel.value;
    updatePreview();
    scheduleApply();
  });

  /* ── Remove button ── */
  const removeBtn = document.createElement("button");
  removeBtn.className = "block-remove";
  removeBtn.textContent = "×";
  removeBtn.title = "Remove block";
  removeBtn.addEventListener("click", () => removeBlock(idx));

  item.append(handle, badge, contentEl, colorSel, fontSel, removeBtn);
  return item;
}

/* ── PREVIEW ──────────────────────────────────────────── */

function updatePreview() {
  const previewEl = document.getElementById("preview");
  const parts = [];

  for (const block of blocks) {
    const colorHex = useColors ? (COLORS[block.color]?.hex ?? null) : null;
    const font = block.font || "normal";
    let text = "";

    if (block.type === "text")     text = block.value.trim();
    if (block.type === "artist")   text = EXAMPLE.artist;
    if (block.type === "title")    text = EXAMPLE.title;
    if (block.type === "time")     text = EXAMPLE.time;
    if (block.type === "duration") text = EXAMPLE.duration;

    if (!text) continue;

    const styled = applyFont(text, font);
    parts.push(colorHex
      ? `<span style="color:${colorHex}">${styled}</span>`
      : styled);
  }

  const cmd = useColors ? "playerchatwheel CW.Agree" : "say";
  previewEl.innerHTML = parts.length
    ? `${cmd} "${parts.join(" ")}"`
    : `<span style="color:#444">— no blocks —</span>`;
}

/* ── COLOR TOGGLE ─────────────────────────────────────── */

function onToggleColors() {
  useColors = document.getElementById("use-colors").checked;
  document.getElementById("warning-box").style.display = useColors ? "" : "none";
  document.querySelectorAll(".block-color-select").forEach(sel => {
    sel.style.display = useColors ? "" : "none";
  });
  updatePreview();
}

/* ── AUTO-SAVE (debounced) ────────────────────────────── */

let _applyTimer = null;

function scheduleApply(userInitiated = true) {
  if (userInitiated) markCustom();
  clearTimeout(_applyTimer);
  setStatus("Unsaved changes...", "warn");
  _applyTimer = setTimeout(() => applyConfig(true), 1000);
}

async function applyConfig(auto = false) {
  clearTimeout(_applyTimer);
  try {
    const res  = await fetch("/config", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ blocks, use_colors: useColors }),
    });
    const data = await res.json();
    if (data.used_placeholder) {
      setStatus("Saved — waiting for a track from SoundCloud", "ok");
    } else {
      setStatus(auto ? "Auto-saved ✓" : "Saved!", "ok");
    }
  } catch {
    setStatus("Save failed — is the server running?", "error");
  }
  setTimeout(() => setStatus("", ""), 3000);
}

function setStatus(msg, type) {
  const el = document.getElementById("status");
  el.textContent = msg;
  el.className = type === "ok"    ? "status ok"
               : type === "error" ? "status error"
               : type === "warn"  ? "status warn"
               : "status";
}

/* ── LOAD CONFIG FROM SERVER ──────────────────────────── */

async function loadConfig() {
  try {
    const res = await fetch("/config");
    const d   = await res.json();

    blocks    = d.blocks     ?? [];
    useColors = d.use_colors ?? false;

    document.getElementById("use-colors").checked = useColors;
    document.getElementById("warning-box").style.display = useColors ? "" : "none";

    renderBlocks();
    updatePreview();

    // Restore active preset from localStorage (survives page refresh)
    const savedPreset = localStorage.getItem('cs2np_active_preset');
    if (savedPreset && activePreset === null) activePreset = savedPreset;

    const last = d.last_artist
      ? `${d.last_artist} - ${d.last_song}`
      : (d.last_song || "—");
    const timeStr = (d.last_time && d.last_duration)
      ? `  [${d.last_time} / ${d.last_duration}]`
      : d.last_time ? `  [${d.last_time}]` : "";
    document.getElementById("last-song").textContent = last + timeStr;
  } catch (e) {
    console.warn("Could not load config:", e);
  }
}

/* ── PRESETS ──────────────────────────────────────────── */

function markCustom() {
  if (activePreset !== null) {
    activePreset = null;
    renderPresets(null);
  }
}

async function loadPresetsFromServer() {
  try {
    const res = await fetch("/presets");
    return await res.json();
  } catch (e) {
    console.error("Failed to load presets:", e);
    return {};
  }
}

async function renderPresets(activeName) {
  const presets = await loadPresetsFromServer();
  const list    = document.getElementById("preset-list");
  list.innerHTML = "";

  const names = Object.keys(presets);
  if (names.length === 0) {
    const empty = document.createElement("div");
    empty.className = "preset-empty";
    empty.textContent = "No saved presets";
    list.appendChild(empty);
    return;
  }

  names.forEach(name => {
    const item = document.createElement("div");
    item.className = "preset-item" + (name === activeName ? " active" : "");

    const dot = document.createElement("div");
    dot.className = "preset-dot";

    const nameEl = document.createElement("div");
    nameEl.className = "preset-name";
    nameEl.textContent = name;
    nameEl.title = name;

    const loadBtn = document.createElement("button");
    loadBtn.className = "preset-btn-load";
    loadBtn.textContent = name === activeName ? "Active" : "Load";
    loadBtn.disabled = name === activeName;
    loadBtn.addEventListener("click", () => presetLoad(name));

    const delBtn = document.createElement("button");
    delBtn.className = "preset-btn-del";
    delBtn.textContent = "×";
    delBtn.title = "Delete preset";
    delBtn.addEventListener("click", () => presetDelete(name));

    item.append(dot, nameEl, loadBtn, delBtn);
    list.appendChild(item);
  });
}

async function presetSave() {
  const nameInput = document.getElementById("preset-name");
  const name      = nameInput.value.trim();
  if (!name) { nameInput.focus(); return; }

  // Push current blocks to server first (without triggering markCustom)
  await fetch("/config", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ blocks, use_colors: useColors }),
  });

  const res = await fetch("/presets", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
  });

  if (res.ok) {
    activePreset = name;   // mark as active BEFORE re-rendering
    localStorage.setItem('cs2np_active_preset', name);
    nameInput.value = "";
    await renderPresets(activePreset);
    setStatus(`Preset "${name}" saved!`, "ok");
    setTimeout(() => setStatus("", ""), 2000);
  } else {
    setStatus("Failed to save preset.", "error");
    setTimeout(() => setStatus("", ""), 2000);
  }
}

async function presetLoad(name) {
  const res = await fetch(`/presets/${encodeURIComponent(name)}/load`, { method: "POST" });
  if (!res.ok) return;
  const d = await res.json();
  if (!d.config) return;

  blocks    = d.config.blocks     ?? [];
  useColors = d.config.use_colors ?? false;

  document.getElementById("use-colors").checked = useColors;
  document.getElementById("warning-box").style.display = useColors ? "" : "none";

  activePreset = name;
  localStorage.setItem('cs2np_active_preset', name);
  await renderPresets(activePreset);
  renderBlocks();
  updatePreview();
  setStatus(`Loaded preset "${name}"`, "ok");
  setTimeout(() => setStatus("", ""), 2000);
}

async function presetDelete(name) {
  if (!confirm(`Delete preset "${name}"?`)) return;
  await fetch(`/presets/${encodeURIComponent(name)}`, { method: "DELETE" });
  if (activePreset === name) {
    activePreset = null;
    localStorage.removeItem('cs2np_active_preset');
  }
  await renderPresets(activePreset);
}

/* ── EXPORT ───────────────────────────────────────────── */

async function exportPresets() {
  try {
    const presets = await loadPresetsFromServer();
    const blob    = new Blob([JSON.stringify(presets, null, 2)], { type: "application/json" });
    const url     = URL.createObjectURL(blob);
    const a       = document.createElement("a");
    a.href = url; a.download = "nowplaying_presets.json"; a.click();
    URL.revokeObjectURL(url);
  } catch (e) {
    console.error("Failed to export presets:", e);
    alert("Failed to export presets. Please try again.");
  }
}

/* ── IMPORT ───────────────────────────────────────────── */

async function importPresets(file) {
  if (!file) return;
  try {
    const data = JSON.parse(await file.text());
    const res  = await fetch("/presets/import", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    });
    if (!res.ok) {
      throw new Error(`Server returned ${res.status}`);
    }
    const d = await res.json();
    alert(`Imported ${d.imported ?? 0} preset(s).`);
    renderPresets(activePreset);
  } catch (e) {
    console.error("Import failed:", e);
    alert("Import failed: invalid JSON file or server error.");
  }
}

/* ── CHAR PICKER ──────────────────────────────────────── */

function buildCharGrid(gridId, chars) {
  const grid = document.getElementById(gridId);
  if (!grid) return;
  grid.innerHTML = "";
  chars.forEach(ch => {
    const btn = document.createElement("button");
    btn.className = "char-btn";
    btn.textContent = ch;
    btn.title = [...ch].map(c =>
      "U+" + c.codePointAt(0).toString(16).toUpperCase().padStart(4, "0")
    ).join(" ");
    btn.addEventListener("click", (e) => {
      e.preventDefault();
      if (!activeInput) return;
      const start = activeInput.selectionStart;
      const end   = activeInput.selectionEnd;
      activeInput.value = activeInput.value.slice(0, start) + ch + activeInput.value.slice(end);
      activeInput.selectionStart = activeInput.selectionEnd = start + ch.length;
      activeInput.focus();

      // Sync value back to blocks state
      const blockItems = document.querySelectorAll(".block-item");
      blockItems.forEach((item, bi) => {
        const inp = item.querySelector(".block-input");
        if (inp && inp === activeInput) blocks[bi].value = inp.value;
      });

      updatePreview();
      scheduleApply();
    });
    grid.appendChild(btn);
  });
}

/* ── POLL LAST TRACK ──────────────────────────────────── */

setInterval(async () => {
  try {
    const res = await fetch("/config");
    const d   = await res.json();

    const last = d.last_artist
      ? `${d.last_artist} - ${d.last_song}`
      : (d.last_song || "—");
    const timeStr = (d.last_time && d.last_duration)
      ? `  [${d.last_time} / ${d.last_duration}]`
      : d.last_time ? `  [${d.last_time}]` : "";
    document.getElementById("last-song").textContent = last + timeStr;
  } catch {}
}, 3000);

/* ── DRAG OVER EMPTY ZONE ─────────────────────────────── */

document.addEventListener("DOMContentLoaded", () => {
  const list = document.getElementById("block-list");

  list.addEventListener("dragover", (e) => {
    e.preventDefault();
    list.classList.add("drag-over-zone");
  });

  list.addEventListener("dragleave", (e) => {
    if (!list.contains(e.relatedTarget))
      list.classList.remove("drag-over-zone");
  });

  list.addEventListener("drop", (e) => {
    e.preventDefault();
    list.classList.remove("drag-over-zone");
    if (e.target === list || e.target.classList.contains("block-empty")) {
      if (dragSrcIdx !== null && dragSrcIdx !== blocks.length - 1) {
        const moved = blocks.splice(dragSrcIdx, 1)[0];
        blocks.push(moved);
        dragSrcIdx = null;
        renderBlocks();
        updatePreview();
        scheduleApply();
      }
    }
    dragSrcIdx = null;
  });
});

/* ── INIT ─────────────────────────────────────────────── */

document.getElementById("use-colors").addEventListener("change", onToggleColors);
document.getElementById("btn-apply").addEventListener("click", () => applyConfig(false));
document.getElementById("btn-preset-save").addEventListener("click", presetSave);
document.getElementById("btn-export").addEventListener("click", exportPresets);
document.getElementById("import-file").addEventListener("change", (e) => {
  importPresets(e.target.files[0]);
  e.target.value = "";
});
document.getElementById("preset-name").addEventListener("keydown", (e) => {
  if (e.key === "Enter") presetSave();
});

document.querySelectorAll(".palette-btn").forEach(btn => {
  btn.addEventListener("click", () => addBlock(btn.dataset.type));
});

for (const [gridId, chars] of Object.entries(CHAR_CATS)) {
  buildCharGrid(gridId, chars);
}

loadConfig();
renderPresets(activePreset);

/* ═══════════════════════════════════════════════════════
   CHAT TRIGGERS
   ═══════════════════════════════════════════════════════ */

let _lastEventId = null;  // track newest event to avoid re-showing

/* ── Watcher status ───────────────────────────────────── */

async function updateWatcherStatus() {
  try {
    const res = await fetch("/watcher/status");
    const d   = await res.json();
    const dot  = document.getElementById("watcher-dot");
    const text = document.getElementById("watcher-status-text");

    if (!d.running) {
      dot.className  = "watcher-dot error";
      text.textContent = d.reason || "Watcher not running";
    } else if (!d.log_exists) {
      dot.className  = "watcher-dot warn";
      text.textContent = `Log not found — start CS2 with -condebug -noclearlog`;
    } else {
      dot.className  = "watcher-dot ok";
      text.textContent = `Watching: ${d.log_path.split(/[\\/]/).slice(-3).join("/")}`;
    }
  } catch {
    const dot = document.getElementById("watcher-dot");
    if (dot) { dot.className = "watcher-dot error"; }
  }
}

/* ── Trigger list ─────────────────────────────────────── */

async function loadTriggers() {
  try {
    const res  = await fetch("/watcher/triggers");
    const list = await res.json();
    renderTriggers(list);
  } catch (e) {
    console.warn("Could not load triggers:", e);
  }
}

function renderTriggers(triggers) {
  const container = document.getElementById("trigger-list");
  container.innerHTML = "";

  if (!triggers.length) {
    const empty = document.createElement("div");
    empty.className = "trigger-empty";
    empty.textContent = "No triggers defined";
    container.appendChild(empty);
    return;
  }

  triggers.forEach(t => {
    const row = document.createElement("div");
    row.className = "trigger-item" + (t.enabled ? "" : " disabled");

    const label = document.createElement("span");
    label.className = "trigger-label-text";
    label.textContent = t.label;
    label.title = t.label;

    const pattern = document.createElement("span");
    pattern.className = "trigger-pattern-text";
    pattern.textContent = t.regex ? `/${t.regex}/` : `"${t.pattern}"`;
    pattern.title = t.regex ? `regex: ${t.regex}` : `match: ${t.pattern}`;

    // Strip prefix badge
    let prefixBadge = null;
    if (t.strip_prefix) {
      prefixBadge = document.createElement("span");
      prefixBadge.className = "trigger-prefix-badge";
      prefixBadge.textContent = `strip: "${t.strip_prefix}"`;
      prefixBadge.title = `Query = everything after "${t.strip_prefix}"`;
    }

    const toggleBtn = document.createElement("button");
    toggleBtn.className = "trigger-toggle";
    toggleBtn.textContent = t.enabled ? "ON" : "OFF";
    toggleBtn.addEventListener("click", () => toggleTrigger(t.id, !t.enabled));

    const delBtn = document.createElement("button");
    delBtn.className = "trigger-del";
    delBtn.textContent = "×";
    delBtn.title = "Delete trigger";
    delBtn.addEventListener("click", () => deleteTrigger(t.id));

    if (prefixBadge) row.append(label, pattern, prefixBadge, toggleBtn, delBtn);
    else row.append(label, pattern, toggleBtn, delBtn);
    container.appendChild(row);
  });
}

async function addTrigger() {
  const label        = document.getElementById("trigger-label").value.trim();
  const pattern      = document.getElementById("trigger-pattern").value.trim();
  const stripPrefix  = document.getElementById("trigger-strip-prefix").value.trim();
  const regex        = document.getElementById("trigger-regex").value.trim();

  if (!label || (!pattern && !regex)) return;

  await fetch("/watcher/triggers", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ label, pattern, regex, strip_prefix: stripPrefix }),
  });

  document.getElementById("trigger-label").value        = "";
  document.getElementById("trigger-pattern").value      = "";
  document.getElementById("trigger-strip-prefix").value = "";
  document.getElementById("trigger-regex").value        = "";
  await loadTriggers();
}

async function toggleTrigger(id, enabled) {
  const res = await fetch(`/watcher/triggers/${encodeURIComponent(id)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ enabled }),
  });
  if (res.ok) {
    const d = await res.json();
    renderTriggers(d.triggers);
  } else {
    await loadTriggers();
  }
}

async function deleteTrigger(id) {
  if (!confirm("Delete this trigger?")) return;
  const res = await fetch(`/watcher/triggers/${encodeURIComponent(id)}`, { method: "DELETE" });
  if (res.ok) {
    const d = await res.json();
    renderTriggers(d.triggers);
  } else {
    await loadTriggers();
  }
}

/* ── Events log ───────────────────────────────────────── */

async function pollEvents() {
  try {
    const res    = await fetch("/watcher/events?limit=30");
    const events = await res.json();
    if (!events.length) return;

    // Only render if there's a new event
    const newestId = events[0]?.id;
    if (newestId === _lastEventId) return;
    _lastEventId = newestId;

    renderEvents(events);
  } catch {}
}

function renderEvents(events) {
  const log = document.getElementById("events-log");
  log.innerHTML = "";

  if (!events.length) {
    const empty = document.createElement("div");
    empty.className = "events-empty";
    empty.textContent = "No events yet";
    log.appendChild(empty);
    return;
  }

  events.forEach(e => {
    const row = document.createElement("div");
    row.className = "event-row";

    const t = document.createElement("span");
    t.className = "event-time";
    t.textContent = e.timestamp ? e.timestamp.split(" - ")[1] : "??:??:??";

    const trig = document.createElement("span");
    trig.className = "event-trigger";
    trig.textContent = `[${e.trigger_label}]`;

    const player = document.createElement("span");
    player.className = "event-player";
    player.textContent = e.player ? `${e.player}:` : "";

    const msg = document.createElement("span");
    msg.className = "event-msg";
    msg.textContent = e.message;

    row.append(t, trig, player, msg);
    log.appendChild(row);
  });
}

/* ── Init triggers ────────────────────────────────────── */

document.getElementById("btn-trigger-add").addEventListener("click", addTrigger);

["trigger-label", "trigger-pattern", "trigger-strip-prefix", "trigger-regex"].forEach(id => {
  document.getElementById(id).addEventListener("keydown", (e) => {
    if (e.key === "Enter") addTrigger();
  });
});

// Poll watcher status every 5s, events every 1s
updateWatcherStatus();
loadTriggers();
setInterval(updateWatcherStatus, 5000);
setInterval(pollEvents, 1000);

/* ═══════════════════════════════════════════════════════
   BAN WORDS
   ═══════════════════════════════════════════════════════ */

async function loadBanWords() {
  try {
    const res   = await fetch("/watcher/banwords");
    const words = await res.json();
    renderBanWords(words);
  } catch (e) {
    console.warn("Could not load ban words:", e);
  }
}

function renderBanWords(words) {
  const container = document.getElementById("banwords-list");
  container.innerHTML = "";

  if (!words.length) {
    const empty = document.createElement("span");
    empty.className = "banwords-empty";
    empty.textContent = "No ban words";
    container.appendChild(empty);
    return;
  }

  words.forEach(word => {
    const chip = document.createElement("div");
    chip.className = "banword-chip";

    const text = document.createTextNode(word);

    const del = document.createElement("button");
    del.className = "banword-chip-del";
    del.textContent = "×";
    del.title = `Remove "${word}"`;
    del.addEventListener("click", () => deleteBanWord(word));

    chip.append(text, del);
    container.appendChild(chip);
  });
}

async function addBanWord() {
  const input = document.getElementById("banword-input");
  const word  = input.value.trim();
  if (!word) { input.focus(); return; }

  await fetch("/watcher/banwords", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ word }),
  });

  input.value = "";
  await loadBanWords();
}

async function deleteBanWord(word) {
  await fetch(`/watcher/banwords/${encodeURIComponent(word)}`, { method: "DELETE" });
  await loadBanWords();
}

document.getElementById("btn-banword-add").addEventListener("click", addBanWord);
document.getElementById("banword-input").addEventListener("keydown", (e) => {
  if (e.key === "Enter") addBanWord();
});

loadBanWords();