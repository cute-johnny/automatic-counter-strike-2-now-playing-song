// ==UserScript==
// @name         CS2 Chat Triggers & Now Playing → YouTube
// @namespace    http://tampermonkey.net/
// @version      2.0
// @description  Listens for CS2 queue/chat playback commands, auto-plays YouTube videos, auto-advances the queue when a video ends or exceeds max duration, and sends Now Playing track/time back to CS2.
// @author       You
// @match        https://www.youtube.com/*
// @run-at       document-idle
// @grant        GM_xmlhttpRequest
// @grant        GM_getValue
// @grant        GM_setValue
// @connect      localhost
// ==/UserScript==

(function () {
    'use strict';

    const BASE_URL    = 'http://localhost:9999';
    const EVENTS_URL  = `${BASE_URL}/watcher/events?limit=5`;
    const YT_SEARCH   = `${BASE_URL}/youtube/search`;
    const SONG_URL    = `${BASE_URL}/song`;
    const TIME_URL    = `${BASE_URL}/time`;
    const ENDED_URL   = `${BASE_URL}/queue/ended`;

    const POLL_EVENTS_MS = 1200;
    const SONG_POLL_MS   = 2000;
    const TIME_POLL_MS   = 1000;

    const KEY_LAST_EVENT  = 'cs2_yt_last_event_id';
    const KEY_INITIALIZED = 'cs2_yt_initialized';

    function lastEventId()        { return GM_getValue(KEY_LAST_EVENT, ''); }
    function markEventHandled(id) { GM_setValue(KEY_LAST_EVENT, id); }

    let _maxDurationSec   = 0;
    let _endedNotifiedFor = null;

    // ─────────────────────────────────────────────────────────
    //  ON-SCREEN STATUS BADGE
    // ─────────────────────────────────────────────────────────

    let _badgeEl    = null;
    let _badgeTimer = null;

    function badge(text, color = '#ff6b35', ms = 4000) {
        if (!document.body) return;
        if (!_badgeEl) {
            _badgeEl = document.createElement('div');
            _badgeEl.style.cssText = [
                'position:fixed', 'bottom:24px', 'right:24px',
                'padding:10px 16px', 'border-radius:8px',
                "font-family:'Segoe UI',system-ui,sans-serif",
                'font-size:13px', 'font-weight:600',
                'z-index:2147483647', 'pointer-events:none',
                'background:#0f0f13ee', 'border:1px solid #ff6b35',
                'box-shadow:0 4px 16px rgba(0,0,0,0.6)',
                'transition:opacity 0.25s ease',
            ].join(';');
            document.body.appendChild(_badgeEl);
        }
        _badgeEl.style.opacity     = '1';
        _badgeEl.style.color       = color;
        _badgeEl.style.borderColor = color;
        _badgeEl.textContent       = text;
        clearTimeout(_badgeTimer);
        _badgeTimer = setTimeout(() => {
            if (_badgeEl) _badgeEl.style.opacity = '0';
        }, ms);
    }

    // ─────────────────────────────────────────────────────────
    //  YOUTUBE SPA NAVIGATION & AUTOPLAY
    // ─────────────────────────────────────────────────────────

    function navigateAndPlayVideo(videoId, queryLabel) {
        _endedNotifiedFor = null;
        const watchPath = `/watch?v=${videoId}`;
        const fullUrl   = `https://www.youtube.com${watchPath}`;

        badge(`▶ Playing: ${queryLabel}`, '#4caf50', 6000);
        console.log(`[CS2→YT] Navigating to ${fullUrl} (${queryLabel})`);

        try {
            const a = document.createElement('a');
            a.className = 'yt-simple-endpoint style-scope ytd-app';
            a.href = watchPath;
            a.style.display = 'none';
            document.body.appendChild(a);
            a.click();
            setTimeout(() => a.remove(), 500);

            setTimeout(() => {
                if (!window.location.search.includes(videoId)) {
                    window.location.assign(fullUrl);
                } else {
                    ensureVideoPlaying();
                }
            }, 800);
        } catch (_) {
            window.location.assign(fullUrl);
        }
    }

    function ensureVideoPlaying() {
        let attempts = 0;
        const timer = setInterval(() => {
            attempts++;
            const video = document.querySelector('video.html5-main-video') || document.querySelector('video');
            if (video) {
                attachVideoEndedListener(video);
                if (video.paused && !video.ended) {
                    video.play().catch(err => {
                        console.warn('[CS2→YT] Autoplay blocked by browser until first interaction:', err);
                    });
                } else if (!video.paused) {
                    clearInterval(timer);
                }
            }
            if (attempts >= 20) clearInterval(timer);
        }, 300);
    }

    function notifySongEnded(reason = 'ended') {
        const curUrl = window.location.href;
        if (_endedNotifiedFor === curUrl) return;
        _endedNotifiedFor = curUrl;

        console.log(`[CS2→YT] Track finished (${reason}) — checking queue for next song`);
        GM_xmlhttpRequest({
            method:  'POST',
            url:     ENDED_URL,
            headers: { 'Content-Type': 'application/json' },
            data:    JSON.stringify({ reason }),
            onload(res) {
                if (res.status === 200) {
                    try {
                        const d = JSON.parse(res.responseText);
                        if (d.advanced && d.song) {
                            badge(`⏭ Auto-playing next: ${d.song.title}`, '#6bd4f5', 4000);
                        }
                    } catch (_) {}
                }
            },
        });
    }

    function attachVideoEndedListener(video) {
        if (!video || video._cs2EndedHooked) return;
        video._cs2EndedHooked = true;
        video.addEventListener('ended', () => {
            notifySongEnded('video_ended');
        });
    }

    window.addEventListener('yt-navigate-finish', () => {
        if (window.location.pathname === '/watch') {
            _endedNotifiedFor = null;
            ensureVideoPlaying();
        }
    });

    // ─────────────────────────────────────────────────────────
    //  RESOLVE QUERY → VIDEO ID & PLAY
    // ─────────────────────────────────────────────────────────

    function searchAndPlay(query, preResolvedVideoId) {
        if (preResolvedVideoId) {
            navigateAndPlayVideo(preResolvedVideoId, query);
            return;
        }
        badge(`🔍 Searching YT: ${query}`, '#ff6b35', 4000);
        GM_xmlhttpRequest({
            method:  'GET',
            url:     `${YT_SEARCH}?q=${encodeURIComponent(query)}`,
            headers: { Accept: 'application/json' },
            timeout: 8000,
            onload(res) {
                if (res.status === 422) {
                    badge(`⏭ Skipped "${query}" (exceeds max song duration)`, '#ff9800', 4000);
                    return;
                }
                if (res.status !== 200) {
                    badge(`❌ No YouTube results for "${query}"`, '#f44336', 4000);
                    return;
                }
                try {
                    const data = JSON.parse(res.responseText);
                    if (typeof data.max_duration_sec === 'number') {
                        _maxDurationSec = data.max_duration_sec;
                    }
                    if (data && data.video_id) {
                        navigateAndPlayVideo(data.video_id, query);
                    } else {
                        badge(`❌ No video found`, '#f44336', 4000);
                    }
                } catch (e) {
                    console.error('[CS2→YT] JSON parse error:', e);
                }
            },
            onerror() {
                badge('❌ Server error searching YouTube', '#f44336', 4000);
            },
        });
    }

    // ─────────────────────────────────────────────────────────
    //  POLL DISPATCHED PLAYBACK COMMANDS
    // ─────────────────────────────────────────────────────────

    let _pollInFlight = false;

    function pollEvents() {
        if (_pollInFlight) return;
        _pollInFlight = true;

        GM_xmlhttpRequest({
            method:  'GET',
            url:     EVENTS_URL,
            headers: { Accept: 'application/json' },
            timeout: 5000,
            onload(res) {
                _pollInFlight = false;
                if (res.status !== 200) return;

                let events;
                try { events = JSON.parse(res.responseText); }
                catch (_) { return; }

                // On first load, mark any existing play event as seen so we don't replay stale history
                if (!GM_getValue(KEY_INITIALIZED, false)) {
                    GM_setValue(KEY_INITIALIZED, true);
                    if (Array.isArray(events) && events.length && events[0]?.id) {
                        markEventHandled(events[0].id);
                    }
                    return;
                }

                if (!Array.isArray(events) || !events.length) return;

                const evt = events[0];
                if (!evt || !evt.id) return;
                if (evt.id === lastEventId()) return;

                const query = (evt.search_query || '').trim()
                           || (evt.message      || '').trim();
                if (!query) return;

                markEventHandled(evt.id);
                console.log(`[CS2→YT] Dispatched play [${evt.trigger_label}] from ${evt.player}: "${query}"`);
                searchAndPlay(query, evt.video_id || '');
            },
            onerror()   { _pollInFlight = false; },
            ontimeout() { _pollInFlight = false; },
        });
    }

    // ─────────────────────────────────────────────────────────
    //  SCRAPE YOUTUBE NOW PLAYING → CS2 CFG
    // ─────────────────────────────────────────────────────────

    let _lastSentKey      = null;
    let _lastSentTime     = null;
    let _lastSentDuration = null;

    function formatSeconds(sec) {
        if (!Number.isFinite(sec) || sec < 0) return null;
        const total = Math.floor(sec);
        const hrs   = Math.floor(total / 3600);
        const mins  = Math.floor((total % 3600) / 60);
        const secs  = String(total % 60).padStart(2, '0');
        return hrs > 0
            ? `${hrs}:${String(mins).padStart(2, '0')}:${secs}`
            : `${mins}:${secs}`;
    }

    function getYouTubeSongInfo() {
        if (window.location.pathname !== '/watch') return { title: null, artist: '' };

        const titleEl = (
            document.querySelector('ytd-watch-metadata h1 yt-formatted-string') ||
            document.querySelector('#title h1 yt-formatted-string') ||
            document.querySelector('.ytp-title-link')
        );
        const channelEl = (
            document.querySelector('ytd-watch-metadata ytd-channel-name a') ||
            document.querySelector('#owner #channel-name a')
        );

        let title  = titleEl   ? (titleEl.textContent   || '').trim() : '';
        let artist = channelEl ? (channelEl.textContent || '').trim() : '';

        if (!title && document.title) {
            title = document.title.replace(/^\(\d+\)\s*/, '').replace(/\s*-\s*YouTube$/i, '').trim();
        }

        artist = artist.replace(/\s*-\s*Topic$/i, '').trim();

        return { title: title || null, artist };
    }

    function postJson(url, payload, onLoad) {
        GM_xmlhttpRequest({
            method:  'POST',
            url:     url,
            headers: { 'Content-Type': 'application/json' },
            data:    JSON.stringify(payload),
            onload(res) {
                if (onLoad && res.status === 200) {
                    try { onLoad(JSON.parse(res.responseText)); } catch (_) {}
                }
            },
        });
    }

    function checkSong() {
        const { title, artist } = getYouTubeSongInfo();
        if (!title) return;

        const key = `${artist}||${title}`;
        if (key === _lastSentKey) return;

        _lastSentKey      = key;
        _lastSentTime     = null;
        _lastSentDuration = null;
        console.log(`[CS2→YT] Now Playing: "${artist ? artist + ' - ' : ''}${title}"`);
        postJson(SONG_URL, { title, artist }, (resp) => {
            if (typeof resp.max_duration_sec === 'number') {
                _maxDurationSec = resp.max_duration_sec;
            }
        });
    }

    function sendTime() {
        if (window.location.pathname !== '/watch') return;
        const video = document.querySelector('video.html5-main-video') || document.querySelector('video');
        if (!video) return;

        attachVideoEndedListener(video);

        if (video.ended) {
            notifySongEnded('video_ended');
            return;
        }

        const current_time = formatSeconds(video.currentTime);
        const duration     = formatSeconds(video.duration) || '';
        if (!current_time) return;

        if (current_time === _lastSentTime && duration === _lastSentDuration) return;
        _lastSentTime     = current_time;
        _lastSentDuration = duration;

        postJson(TIME_URL, { current_time, duration }, (resp) => {
            if (typeof resp.max_duration_sec === 'number') {
                _maxDurationSec = resp.max_duration_sec;
            }
            if (resp.auto_skipped) {
                badge('⏭ Max song length reached — advancing queue', '#ff9800', 4000);
            }
        });
    }

    // ─────────────────────────────────────────────────────────
    //  START
    // ─────────────────────────────────────────────────────────

    console.log('[CS2→YT] Userscript v2.0 active — Queue & Auto-Advance enabled');
    GM_setValue(KEY_INITIALIZED, false);
    setInterval(pollEvents, POLL_EVENTS_MS);
    setInterval(checkSong,  SONG_POLL_MS);
    setInterval(sendTime,   TIME_POLL_MS);

})();
