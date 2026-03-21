// ==UserScript==
// @name         SoundCloud → CS2 Now Playing
// @namespace    http://tampermonkey.net/
// @version      3.0
// @description  Scrapes track title, artist, playback time and duration from SoundCloud and sends to local Python server
// @author       You
// @match        https://soundcloud.com/*
// @grant        GM_xmlhttpRequest
// @connect      localhost
// ==/UserScript==

(function () {
    'use strict';

    const SONG_URL = 'http://localhost:9999/song';
    const TIME_URL = 'http://localhost:9999/time';

    const SONG_INTERVAL_MS = 2000;  // how often to check for a track change
    const TIME_INTERVAL_MS = 3000;  // how often to send playback time (required)

    let lastSentKey = null;

    // ── Scraping ──────────────────────────────────────────

    function getTitle() {
        const el = document.querySelector('a.playbackSoundBadge__titleLink');
        return el ? (el.getAttribute('title') || '').trim() : null;
    }

    function getArtist() {
        const el = document.querySelector('a.playbackSoundBadge__lightLink');
        if (!el) return '';
        return (el.getAttribute('title') || el.textContent || '').trim();
    }

    function getCurrentTime() {
        // aria-hidden span inside playbackTimeline__timePassed — contains e.g. "1:55"
        const el = document.querySelector('.playbackTimeline__timePassed span[aria-hidden="true"]');
        return el ? el.textContent.trim() : null;
    }

    function getDuration() {
        // aria-hidden span inside playbackTimeline__duration — contains e.g. "3:18"
        const el = document.querySelector('.playbackTimeline__duration span[aria-hidden="true"]');
        return el ? el.textContent.trim() : null;
    }

    // ── HTTP helper ───────────────────────────────────────

    function post(url, data) {
        GM_xmlhttpRequest({
            method:  'POST',
            url:     url,
            headers: { 'Content-Type': 'application/json' },
            data:    JSON.stringify(data),
            onerror: () => console.warn('[CS2 NowPlaying] Connection error. Is the Python server running?'),
        });
    }

    // ── Track change check (every 2s) ─────────────────────

    function checkSong() {
        const title = getTitle();
        if (!title) return;

        const artist = getArtist();
        const key    = `${artist}||${title}`;
        if (key === lastSentKey) return;

        lastSentKey = key;
        console.log(`[CS2 NowPlaying] New track: "${artist} - ${title}"`);
        post(SONG_URL, { title, artist });
    }

    // ── Playback time update (every 3s) ───────────────────

    function sendTime() {
        const current_time = getCurrentTime();
        const duration     = getDuration();
        if (!current_time) return;  // nothing playing
        post(TIME_URL, { current_time, duration: duration || '' });
    }

    // ── Start ─────────────────────────────────────────────

    console.log('[CS2 NowPlaying] Script active — monitoring SoundCloud...');
    setInterval(checkSong, SONG_INTERVAL_MS);
    setInterval(sendTime,  TIME_INTERVAL_MS);

})();