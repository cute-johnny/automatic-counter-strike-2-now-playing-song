// ==UserScript==
// @name         CS2 Chat Events → SoundCloud
// @namespace    http://tampermonkey.net/
// @version      5.6
// @description  Listens for CS2 chat triggers, searches SoundCloud and plays first result. 30s cooldown.
// @author       You
// @match        https://soundcloud.com/*
// @run-at       document-idle
// @grant        GM_xmlhttpRequest
// @grant        GM_getValue
// @grant        GM_setValue
// @connect      localhost
// ==/UserScript==

(function () {
    'use strict';

    const SERVER_URL      = window.CS2_SERVER_URL || 'http://localhost:9999/watcher/events?limit=5';
    const QUEUE_URL       = window.CS2_QUEUE_URL  || 'http://localhost:9999/queue';
    const POLL_MS         = 1500;
    const PLAY_TIMEOUT_MS = 5000;
    const PLAY_POLL_MS    = 200;

    // ─────────────────────────────────────────────────────────
    //  PERSISTENT STORAGE
    // ─────────────────────────────────────────────────────────

    const KEY_LAST_EVENT  = 'cs2_last_event_id';
    const KEY_PENDING_Q   = 'cs2_pending_query';
    const KEY_LAST_PLAYED = 'cs2_last_played';

    const COOLDOWN_MS = 30000;

    function lastEventId()    { return GM_getValue(KEY_LAST_EVENT,  ''); }
    function pendingQuery()   { return GM_getValue(KEY_PENDING_Q,   ''); }
    function lastPlayedTime() { return GM_getValue(KEY_LAST_PLAYED, 0);  }

    function markEventHandled(id) { GM_setValue(KEY_LAST_EVENT,  id);  }
    function setPendingQuery(q)   { GM_setValue(KEY_PENDING_Q,   q);   }
    function clearPendingQuery()  { GM_setValue(KEY_PENDING_Q,   '');  }
    function markPlayed()         { GM_setValue(KEY_LAST_PLAYED, Date.now()); }

    function isOnCooldown() {
        return Date.now() - lastPlayedTime() < COOLDOWN_MS;
    }

    // ─────────────────────────────────────────────────────────
    //  REACT CLICK
    // ─────────────────────────────────────────────────────────

    const REACT_CLICK_EVENTS = ['pointerdown','mousedown','pointerup','mouseup','click'];

    function reactClick(el) {
        REACT_CLICK_EVENTS.forEach(type => {
            el.dispatchEvent(new MouseEvent(type, { bubbles: true, cancelable: true }));
        });
    }

    // ─────────────────────────────────────────────────────────
    //  QUEUE
    // ─────────────────────────────────────────────────────────

    function addToQueue(title, artist = '') {
        GM_xmlhttpRequest({
            method: 'POST',
            url: QUEUE_URL,
            headers: { 'Content-Type': 'application/json' },
            data: JSON.stringify({ title, artist, source: 'userscript' }),
            onload(res) {
                if (res.status === 429) {
                    badge('❌ Queue full', '#ff4444', 3000);
                } else if (res.status === 200) {
                    badge(`✓ Added to queue: ${title}`, '#44aa44', 2000);
                } else {
                    badge('❌ Failed to add to queue', '#ff4444', 3000);
                }
            },
            onerror() {
                badge('❌ Server error', '#ff4444', 3000);
            }
        });
    }

    window.CS2AddToQueue = addToQueue;

    // ─────────────────────────────────────────────────────────
    //  PLAY FIRST RESULT
    // ─────────────────────────────────────────────────────────

    function findAnyPlayPauseButton() {
        return (
            document.querySelector('.searchList__item:first-child .playButton') ||
            document.querySelector('.soundList__item:first-child  .playButton') ||
            document.querySelector('.soundTitle__playButton a.sc-button-play')  ||
            document.querySelector('.soundTitle__playButton a[title="Play"]')   ||
            document.querySelector('.soundTitle__playButton a[title="Pause"]')  ||
            document.querySelector('a.playButton.sc-button-play')               ||
            document.querySelector('button.playButton, a.playButton')           ||
            null
        );
    }

    let _playTimer = null;

    function startPlaying(query) {
        if (_playTimer) { clearInterval(_playTimer); _playTimer = null; }

        const deadline = Date.now() + PLAY_TIMEOUT_MS;

        _playTimer = setInterval(() => {

            if (isOnCooldown()) {
                clearInterval(_playTimer); _playTimer = null;
                clearPendingQuery();
                return;
            }

            const btn = findAnyPlayPauseButton();

            if (btn) {
                const titleAttr = (btn.getAttribute('title') || '').trim().toLowerCase();
                console.log(`[CS2→SC] Found button, title="${titleAttr}"`);

                if (titleAttr.includes('pause')) {
                    clearInterval(_playTimer); _playTimer = null;
                    markPlayed();
                    clearPendingQuery();
                    console.log(`[CS2→SC] Song already playing for "${query}".`);
                    badge(`▶ Playing: ${query}`, '#4caf50', 10000);

                } else if (titleAttr.includes('play') || titleAttr === '') {
                    clearInterval(_playTimer); _playTimer = null;
                    markPlayed();
                    clearPendingQuery();
                    reactClick(btn);
                    console.log(`[CS2→SC] ▶ reactClick fired for "${query}".`);
                    badge(`▶ Playing: ${query}`, '#4caf50', 10000);
                }
                return;
            }

            if (Date.now() > deadline) {
                clearInterval(_playTimer); _playTimer = null;
                clearPendingQuery();
                console.warn(`[CS2→SC] Timed out — play button never appeared.`);
                badge('❌ No results found', '#f44336', 4000);
            }

        }, PLAY_POLL_MS);
    }

    // ─────────────────────────────────────────────────────────
    //  SEARCH
    // ─────────────────────────────────────────────────────────

    function searchFor(query) {
        const url = `https://soundcloud.com/search/sounds?q=${encodeURIComponent(query)}`;
        console.log(`[CS2→SC] Searching for "${query}" → ${url}`);
        setPendingQuery(query);
        badge(`🔍 Searching: ${query}`, '#ff6b35', 5000);
        try {
            if (document.readyState === 'complete') {
                window.location.assign(url);
            } else {
                window.addEventListener('load', () => {
                    window.location.assign(url);
                }, { once: true });
            }
        } catch (e) {
            console.error('[CS2→SC] Navigation error:', e);
            badge(`❌ Navigation error`, '#ff0000', 3000);
        }
    }

    // ─────────────────────────────────────────────────────────
    //  POLL SERVER
    // ─────────────────────────────────────────────────────────

    let _pollInFlight = false;

    function poll() {
        if (_pollInFlight || pendingQuery() || isOnCooldown()) return;
        _pollInFlight = true;

        GM_xmlhttpRequest({
            method:  'GET',
            url:     SERVER_URL,
            headers: { Accept: 'application/json' },
            timeout: 10000,
            onload(res) {
                _pollInFlight = false;
                if (res.status !== 200) {
                    console.error('[CS2→SC] Server error:', res.status);
                    return;
                }

                let events;
                try { events = JSON.parse(res.responseText); }
                catch (e) {
                    console.error('[CS2→SC] JSON parse error:', e);
                    return;
                }
                if (!events.length) return;

                const evt = events[0];
                if (!evt?.id) return;
                if (evt.id === lastEventId()) return;

                const query = (evt.search_query || '').trim()
                           || (evt.message      || '').trim();
                if (!query) return;

                markEventHandled(evt.id);
                console.log(`[CS2→SC] Chat trigger [${evt.trigger_label}]: "${query}"`);
                searchFor(query);
            },
            onerror(err) {
                _pollInFlight = false;
                console.error('[CS2→SC] Server connection error:', err);
            },
            ontimeout() {
                _pollInFlight = false;
                console.error('[CS2→SC] Server request timeout');
            },
        });
    }

    // ─────────────────────────────────────────────────────────
    //  INIT
    // ─────────────────────────────────────────────────────────

    function init() {
        if (window.__cs2ChatEventsInitialized) return;
        window.__cs2ChatEventsInitialized = true;

        const path = window.location.pathname;
        const pq   = pendingQuery();

        if (pq) {
            if (path.startsWith('/search')) {
                // Full reload landed on search results — play exactly once.
                console.log(`[CS2→SC] Results page — pending query: "${pq}"`);
                startPlaying(pq);
            } else {
                // Stale pendingQuery (user navigated away manually, etc.)
                console.warn(`[CS2→SC] Clearing stale pending query: "${pq}"`);
                clearPendingQuery();
            }
        }

        setInterval(poll, POLL_MS);
        setInterval(keepAliveCheck, KEEPALIVE_POLL_MS);

        // FIX #4: Guard against double-wrapping pushState if init() somehow
        // runs more than once (e.g. TM re-injection edge cases).
        if (!history._cs2patched) {
            history._cs2patched = true;

            // FIX: SPA intercept → force a hard reload instead of calling
            // startPlaying() again, which caused double-click (play → pause).
            // After reload, init() handles playback exactly once via the
            // pendingQuery path above.
            function handleRouteChange() {
                const pq = pendingQuery();
                if (!pq || isOnCooldown()) return;
                if (!window.location.pathname.startsWith('/search')) return;

                console.log(`[CS2→SC] SPA intercept detected — forcing hard reload`);
                window.location.reload();
            }

            const _origPushState = history.pushState.bind(history);
            history.pushState = function (...args) {
                _origPushState(...args);
                handleRouteChange();
            };
            window.addEventListener('popstate', handleRouteChange);
        }
    }

    // ─────────────────────────────────────────────────────────
    //  BADGE
    // ─────────────────────────────────────────────────────────

    let _badgeEl    = null;
    let _badgeTimer = null;

    function badge(text, color, ms) {
        if (!_badgeEl) {
            _badgeEl = document.createElement('div');
            _badgeEl.style.cssText = [
                'position:fixed', 'bottom:80px', 'right:16px',
                'padding:9px 16px', 'border-radius:8px',
                "font-family:'Segoe UI',system-ui,sans-serif",
                'font-size:0.82rem', 'font-weight:600',
                'z-index:2147483647', 'pointer-events:none',
                'background:#1a1a24', 'border:1px solid #ff6b35',
                'box-shadow:0 2px 14px #00000055',
                'transition:opacity 0.3s',
            ].join(';');
            document.body.appendChild(_badgeEl);
        }
        _badgeEl.style.opacity     = '1';
        _badgeEl.style.color       = color;
        _badgeEl.style.borderColor = color;
        _badgeEl.textContent       = text;
        clearTimeout(_badgeTimer);
        _badgeTimer = setTimeout(() => { _badgeEl.style.opacity = '0'; }, ms);
    }

    // ─────────────────────────────────────────────────────────
    //  PLAYBACK KEEPALIVE
    //  FIX #1: Use lastPlayedTime() (GM_getValue, survives navigation)
    //  instead of _scriptPlayedAt (local var, resets to 0 on every reload).
    // ─────────────────────────────────────────────────────────

    const KEEPALIVE_WINDOW_MS = 120000;
    const KEEPALIVE_POLL_MS   = 2000;

    function keepAliveCheck() {
        // FIX: was `Date.now() - _scriptPlayedAt` which was always > 120000
        // after any navigation, making keepalive permanently inactive.
        if (Date.now() - lastPlayedTime() > KEEPALIVE_WINDOW_MS) return;

        const btn = document.querySelector(
            '.playControls__play, ' +
            '.playControls__control.sc-button-play'
        );
        if (!btn) return;

        const title = (btn.getAttribute('title') || btn.getAttribute('aria-label') || '').trim();
        if (title === 'Play current') {
            console.log('[CS2→SC] Keepalive: playback stopped — resuming');
            reactClick(btn);
            badge('▶ Resumed', '#4caf50', 2000);
        }
    }

    // ─────────────────────────────────────────────────────────
    //  START
    // ─────────────────────────────────────────────────────────

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

})();
