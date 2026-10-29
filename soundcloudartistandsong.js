// ==UserScript==
// @name         SoundCloud → CS2 Now Playing
// @namespace    http://tampermonkey.net/
// @version      3.1
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
    const TIME_INTERVAL_MS = 1000;  // how often to check playback time

    let lastSentKey      = null;
    let lastSentTime     = null;
    let lastSentDuration = null;
    let serverNeedsTime  = true;

    // ── Scraping ──────────────────────────────────────────

    function getSongInfo() {
        const titleEl  = document.querySelector('a.playbackSoundBadge__titleLink');
        const artistEl = document.querySelector('a.playbackSoundBadge__lightLink');
        return {
            title:  titleEl  ? (titleEl.getAttribute('title') || '').trim() || null : null,
            artist: artistEl ? (artistEl.getAttribute('title') || artistEl.textContent || '').trim() : '',
        };
    }

    function getTimeInfo() {
        const timeEl = document.querySelector('.playbackTimeline__timePassed span[aria-hidden="true"]');
        const durEl  = document.querySelector('.playbackTimeline__duration span[aria-hidden="true"]');
        return {
            current_time: timeEl ? timeEl.textContent.trim() : null,
            duration:     durEl  ? durEl.textContent.trim()  : null,
        };
    }

    // ── HTTP helper ───────────────────────────────────────

    function post(url, data, onSuccess) {
        GM_xmlhttpRequest({
            method:  'POST',
            url:     url,
            headers: { 'Content-Type': 'application/json' },
            data:    JSON.stringify(data),
            onload:  (res) => {
                if (onSuccess && res.status === 200) {
                    try {
                        onSuccess(JSON.parse(res.responseText));
                    } catch (_) {}
                }
            },
            onerror: () => console.warn('[CS2 NowPlaying] Connection error. Is the Python server running?'),
        });
    }

    // ── Track change check (every 2s) ─────────────────────

    function checkSong() {
        const { title, artist } = getSongInfo();
        if (!title) return;

        const key = `${artist}||${title}`;
        if (key === lastSentKey) return;

        lastSentKey      = key;
        lastSentTime     = null;
        lastSentDuration = null;
        console.log(`[CS2 NowPlaying] New track: "${artist} - ${title}"`);
        post(SONG_URL, { title, artist }, (resp) => {
            if (typeof resp.need_time === 'boolean') {
                serverNeedsTime = resp.need_time;
            }
        });
    }

    // ── Playback time update (only when changed) ──────────

    function sendTime() {
        const { current_time, duration } = getTimeInfo();
        if (!current_time) return;  // nothing playing

        const dur = duration || '';
        // Skip POST if playback is paused / time has not changed
        if (current_time === lastSentTime && dur === lastSentDuration) return;

        lastSentTime     = current_time;
        lastSentDuration = dur;
        post(TIME_URL, { current_time, duration: dur }, (resp) => {
            if (typeof resp.need_time === 'boolean') {
                serverNeedsTime = resp.need_time;
            }
        });
    }

    // ── Start ─────────────────────────────────────────────

    console.log('[CS2 NowPlaying] Script active — monitoring SoundCloud...');
    setInterval(checkSong, SONG_INTERVAL_MS);
    setInterval(sendTime,  TIME_INTERVAL_MS);

})();
