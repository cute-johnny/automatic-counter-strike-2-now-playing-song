# CS2 Now Playing

Automatically detects what you are listening to on SoundCloud and lets you share it in CS2 chat with a single mouse button press.

---

## How it works

```
SoundCloud (browser)
  └─ Tampermonkey script watches the player
       └─ Sends track info to local Python server every 2–3 seconds
            └─ Python server writes nowplaying.cfg
                 └─ You press a side mouse button in CS2
                      └─ CS2 runs the cfg → message appears in chat
```

---

## Requirements

| Requirement | Where to get it |
|---|---|
| Python 3.10 or newer | https://www.python.org/downloads/ |
| Flask (Python library) | `pip install flask` but you will probably need to install it with admin rights |
| Violentmonkey browser extension | https://violentmonkey.github.io/ or  |
| Steam + CS2 installed | https://store.steampowered.com/app/730 |

---

## Installation — step by step

### Step 1 — Download the project files

Place all project files in a folder of your choice, for example:

```
C:\Users\YourName\Documents\cs2nowplaying\
├── server.py
├── templates\
│   └── index.html
└── static\
    ├── style.css
    └── app.js
```

> The `templates\` and `static\` folders **must** be in the same directory as `server.py`. Flask will not find the panel otherwise.

---

### Step 2 — Install Python

1. Go to https://www.python.org/downloads/ and download the latest Python 3 installer.
2. Run the installer. **Make sure to check "Add Python to PATH"** on the first screen.
3. Verify the installation by opening a terminal and running:

```
python --version
```

You should see something like `Python 3.12.0`.

---

### Step 3 — Install Flask

Open a terminal with administrative priviliges (Command Prompt or PowerShell on Windows) and run:

```
pip install flask
```

---

### Step 4 — Install Violentmonkey

1. Open your browser (Chrome, Firefox, or Edge).
2. Go to https://violentmonkey.github.io/ and click **Download** for your browser.
3. Install the extension and make sure it is enabled.

---

### Step 5 — Install the userscript

1. Click the Violentmonkey icon in your browser toolbar.
2. Select **Create a new script**.
3. Delete all the default code in the editor.
4. Open `soundcloud_nowplaying.user.js` from the project files, copy all its contents, and paste them into the Tampermonkey editor.
5. Press **Ctrl+S** (or click **File → Save**).

The script is now active on every SoundCloud page.

---

### Step 6 — Add the CS2 bind

This tells CS2 which key will trigger the message.

**Option A — type it in-game:**

Open the CS2 developer console and type:

```
bind "MOUSE4" "exec nowplaying.cfg"
```

`MOUSE4` is the back side button on most mice. Use `MOUSE5` for the forward side button if you prefer.

**Option B — add it permanently to autoexec.cfg:**

1. Navigate to your CS2 cfg folder. The typical path is:

```
C:\Program Files (x86)\Steam\steamapps\common\Counter-Strike Global Offensive\game\csgo\cfg\
```

2. Open (or create) a file called `autoexec.cfg` and add this line:

```
bind "MOUSE4" "exec nowplaying.cfg"
```

3. In CS2 console, run `exec autoexec` to apply it, or just restart the game.

---

### Step 7 — Start the server

Open a terminal, navigate to the project folder, and run:

```
python server.py
```

On first launch, the server will automatically detect your CS2 installation by reading the Windows registry. If it cannot find it, it will ask you to enter the path manually.

You should see output like:

```
+------------------------------------------------------+
|    CS2 Now Playing  .  SoundCloud -> CFG Edition     |
+------------------------------------------------------+

[12:00:00] [v] Found CS2 cfg folder: C:\...\game\csgo\cfg
[12:00:00] [i] Server / Panel : http://localhost:9999
[12:00:00] [i] CFG file       : C:\...\game\csgo\cfg\nowplaying.cfg
[12:00:00] [i] Waiting for data from SoundCloud...
```

Keep this terminal window open while playing.

---

### Step 8 — Configure the message (optional)

Open http://localhost:9999 in your browser. You will see the **Message Builder** panel.

Here you can:

- **Add blocks** from the palette at the top — TEXT, ARTIST, TITLE, TIME, LENGTH
- **Drag blocks** to reorder them using the `⠿` handle on the left
- **Set a font** for each block using the font dropdown (Unicode styles like Bold, Italic, Script, etc.)
- NOT WORKING --**Enable colors** with the toggle at the top ()--
- **Save presets** and switch between them at any time
- **Export/import presets** as a JSON file for backup

Changes are automatically saved 1 second after your last edit.

---

### Step 9 — Play a song and try it

1. Open SoundCloud or refresh tab with soundcloud in the browser where Violentmonkey is installed.
2. Play any song.
3. The terminal should show something like:

```
[12:01:05] [#] Cartoon Network - Ben 10 (2005) Intro PL
[12:01:05] [>] say "Now Playing: Cartoon Network - Ben 10 (2005) Intro PL"
[12:01:05] [v] CFG updated! Press MOUSE4/MOUSE5 in CS2 to send.
```

4. Switch to CS2 and press your bound mouse button. The message will appear in chat.

---

## File structure

```
cs2nowplaying\
│
├── server.py                   Main Python server
│
├── templates\
│   └── index.html              Web panel HTML structure
│
├── static\
│   ├── style.css               Web panel styles
│   └── app.js                  Web panel logic (drag builder, presets, char picker)
│
├── soundcloud_nowplaying.user.js   Tampermonkey script (install in browser)
│
├── nowplaying_presets.json     Created automatically when you save a preset
└── nowplaying_history.txt      Created automatically — log of every detected track
```

> `nowplaying.cfg` is written directly into your CS2 cfg folder by the server.

---

## Block types

| Block | What it inserts |
|---|---|
| TEXT | Any custom text you type (e.g. `♪`, `Now Playing:`, `[`) |
| ARTIST | Artist name scraped from SoundCloud automatically |
| TITLE | Song title scraped from SoundCloud automatically |
| TIME | Current playback position, e.g. `1:55` — updates every 3 seconds |
| LENGTH | Total song duration, e.g. `3:18` |

You can add as many blocks as you like, in any order. Example layout:

```
[TEXT: "♪"] [ARTIST] [TEXT: "-"] [TITLE] [TEXT: "["] [TIME] [TEXT: "/"] [LENGTH] [TEXT: "]"]
```

Produces: `♪ Cartoon Network - Ben 10 (2005) Intro PL [1:55 / 3:18]`

---

## Font styles

Each block has a font dropdown. These are Unicode character substitutions — no external fonts are needed and they work in CS2 chat.

| Style | Example |
|---|---|
| Normal | Now Playing |
| 𝐁𝐨𝐥𝐝 | 𝐍𝐨𝐰 𝐏𝐥𝐚𝐲𝐢𝐧𝐠 |
| 𝐼𝑡𝑎𝑙𝑖𝑐 | 𝑁𝑜𝑤 𝑃𝑙𝑎𝑦𝑖𝑛𝑔 |
| 𝑩𝒐𝒍𝒅 𝑰𝒕𝒂𝒍𝒊𝒄 | 𝑵𝒐𝒘 𝑷𝒍𝒂𝒚𝒊𝒏𝒈 |
| 𝒮𝒸𝓇𝒾𝓅𝓉 | 𝒩𝑜𝓌 𝒫𝓁𝒶𝓎𝒾𝓃𝑔 |
| 𝓑𝓸𝓵𝓭 𝓢𝓬𝓻𝓲𝓹𝓽 | 𝓝𝓸𝔀 𝓟𝓵𝓪𝔂𝓲𝓷𝓰 |
| 𝔉𝔯𝔞𝔨𝔱𝔲𝔯 | 𝔑𝔬𝔴 𝔓𝔩𝔞𝔶𝔦𝔫𝔤 |
| 𝔻𝕠𝕦𝕓𝕝𝕖 | 𝕹𝖔𝖜 𝕻𝖑𝖆𝖞𝖎𝖓𝖌 |
| 𝙼𝚘𝚗𝚘 | 𝙽𝚘𝚠 𝙿𝚕𝚊𝚢𝚒𝚗𝚐 |
| Ｗｉｄｅ | Ｎｏｗ Ｐｌａｙｉｎｇ |
| Sᴍᴀʟʟ Cᴀᴘs | ɴᴏᴡ ᴘʟᴀʏɪɴɢ |
| Ⓒⓘⓡⓒⓛⓔⓓ | ⓃⓄⓌ ⓅⓁⒶⓎⒾⓃⒼ |
| S̶t̶r̶i̶k̶e̶ | N̶o̶w̶ P̶l̶a̶y̶i̶n̶g̶ |

---

## Presets

You can save multiple message layouts and switch between them at any time from the panel.

- **Save** — type a name and click Save (or press Enter)
- **Load** — click Load next to any preset; it applies immediately
- **Delete** — click × next to any preset
- **Export** — downloads all presets as a `.json` file
- **Import** — loads presets from a previously exported `.json` file; merges with existing ones

Presets are stored in `nowplaying_presets.json` next to `server.py` and survive server restarts.

---

## Colors (advanced)

The color toggle in the panel enables Source Engine color codes. These only work via the `playerchatwheel` command, which means:

- Messages are visible to **team only**, not all players
- There is a cooldown of roughly 10–15 seconds between messages

> **Note:** Valve patched the color method in June 2025. Colors are currently non-functional in CS2. The option remains in the panel — if Valve reverses the patch it will work automatically without any changes needed.

---

## Troubleshooting

**The server cannot find my CS2 installation**

Enter the path to your CS2 cfg folder manually when prompted. It typically looks like:

```
D:\Steam\steamapps\common\Counter-Strike Global Offensive\game\csgo\cfg
```

**The message is not appearing in chat**

- Make sure the server is running (check the terminal)
- Make sure SoundCloud is playing and Tampermonkey is enabled on soundcloud.com
- Check the terminal — it should log `CFG updated!` when a new track is detected
- Verify the bind is set correctly: open CS2 console and type `bind MOUSE4` — it should show `exec nowplaying.cfg`

**Tampermonkey shows a connection error**

The browser script cannot reach the server. Make sure `python server.py` is running and nothing is blocking port 9999 (firewall or antivirus).

**The CFG file has placeholder text (Song Title / Artist)**

This means no track has been detected yet. Open SoundCloud and play something — the file will be overwritten with real data as soon as the Tampermonkey script picks it up.

---

## Quick reference card

```
1. pip install flask (with admin)
2. Install Violentmonkey → add soundcloud_nowplaying.user.js
3. python server.py
4. In CS2 console: bind "MOUSE4" "exec nowplaying.cfg"
5. Play music on SoundCloud or refresh tab with soundcloud
6. Press MOUSE4 in CS2 → message sent
7. Customize at http://localhost:9999
```
