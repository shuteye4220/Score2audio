'use strict';
(() => {
  const $ = (id) => document.getElementById(id);
  const el = (tag, props = {}, ...kids) => {
    const e = document.createElement(tag);
    for (const [k, v] of Object.entries(props)) {
      if (k === 'class') e.className = v;
      else if (k === 'text') e.textContent = v;
      else if (k.startsWith('on')) e.addEventListener(k.slice(2), v);
      else if (v === true) e.setAttribute(k, '');
      else if (v !== false && v != null) e.setAttribute(k, v);
    }
    for (const kid of kids) if (kid != null) e.append(kid);
    return e;
  };

  // ---------- connection to the program ----------
  let TOKEN = new URLSearchParams(location.search).get('t');
  try {
    if (TOKEN) sessionStorage.setItem('score2audio-token', TOKEN);
    else TOKEN = sessionStorage.getItem('score2audio-token');
  } catch (e) { /* storage unavailable */ }
  if (location.search) history.replaceState(null, '', location.pathname);

  async function api(path, opts = {}) {
    opts.headers = Object.assign({ 'X-Token': TOKEN || '' }, opts.headers || {});
    let r;
    try { r = await fetch(path, opts); } catch (e) {
      throw new Error('score2audio is not running. Start score2audio again, then reload this page.');
    }
    if (!r.ok) throw new Error((await r.text()).trim() || r.statusText);
    const ct = r.headers.get('content-type') || '';
    return ct.includes('json') ? r.json() : r.text();
  }

  // ---------- messages ----------
  let statusTimer, annTimer;
  function setStatus(msg) {
    clearTimeout(statusTimer);
    $('status').textContent = '';
    if (msg) statusTimer = setTimeout(() => { $('status').textContent = msg; }, 60);
  }
  function setError(msg) {
    $('error').textContent = '';
    if (msg) setTimeout(() => { $('error').textContent = msg; }, 60);
  }
  function announce(msg) {
    clearTimeout(annTimer);
    $('announcer').textContent = '';
    annTimer = setTimeout(() => { $('announcer').textContent = msg; }, 40);
  }

  // ---------- state ----------
  let score = null;        // { id, meta, model }
  let M = null;            // model
  let lines = [];          // line objects with user settings
  const opts = {
    tempo: 90, from: 1, to: 1, loop: false, countIn: true, metronome: false,
    repeats: true, sayMeasures: false, sayLyrics: false, verse: 0, notesLine: '',
  };
  let factor = 1;          // user tempo / score tempo
  let beatQn0 = 1;         // length of the beat (in quarter notes) at the start

  function beatQn(meas) {
    if (meas.beatType === 8 && meas.beats % 3 === 0 && meas.beats > 3) return 1.5;
    return 4 / meas.beatType;
  }
  function beatName(q) {
    return { 4: 'whole note', 3: 'dotted half note', 2: 'half note', 1.5: 'dotted quarter note', 1: 'quarter note', 0.5: 'eighth note' }[q] || 'quarter note';
  }
  function lineName(l) { return l.label || l.name; }

  function measureIndex(num) {
    const s = String(num).trim();
    const i = M.measures.findIndex((m) => m.n === s);
    if (i >= 0) return i;
    const n = parseInt(s, 10);
    if (isNaN(n)) return -1;
    return Math.max(0, Math.min(M.measures.length - 1, n - 1));
  }
  const lastNumber = () => M.measures[M.measures.length - 1].n;

  function secBetween(a, b) {
    if (b <= a) return 0;
    let s = 0;
    const tm = M.tempo;
    for (let i = 0; i < tm.length; i++) {
      const lo = Math.max(a, tm[i].t);
      const hi = Math.min(b, i + 1 < tm.length ? tm[i + 1].t : Infinity);
      if (hi > lo) s += (hi - lo) * 60 / (tm[i].bpm * factor);
    }
    return s;
  }

  function currentRange() {
    let f = measureIndex(opts.from), t = measureIndex(opts.to);
    if (f < 0) f = 0;
    if (t < 0) t = M.measures.length - 1;
    if (t < f) [f, t] = [t, f];
    return { from: f, to: t };
  }

  // ---------- performance plan ----------
  function buildPlan(range, withCountIn, withClicks, verseIdx) {
    const order = (opts.repeats && M.hasRepeats) ? M.order : M.measures.map((_, i) => i);
    let k0 = order.indexOf(range.from);
    if (k0 < 0) k0 = Math.max(0, order.findIndex((mi) => mi >= range.from));
    let k1 = -1;
    for (let k = k0; k < order.length; k++) if (order[k] === range.to) k1 = k;
    if (k1 < 0) { k1 = k0; for (let k = k0; k < order.length; k++) if (order[k] <= range.to) k1 = k; }

    const clicks = [];
    let sec = 0;
    const first = M.measures[order[k0]];
    if (withCountIn) {
      const bq = beatQn(first);
      const n = Math.max(1, Math.round(first.beats * (4 / first.beatType) / bq));
      const beatSec = secBetween(first.t, first.t + bq) || 60 / opts.tempo;
      for (let i = 0; i < n; i++) clicks.push({ s: i * beatSec, accent: i === 0 });
      sec = n * beatSec;
    }
    const countInSec = sec;
    const perf = [];
    for (let k = k0; k <= k1; k++) {
      const mi = order[k], meas = M.measures[mi];
      const dur = secBetween(meas.t, meas.t + meas.d);
      perf.push({ mi, sec, dur });
      if (withClicks) {
        const bq = beatQn(meas);
        const n = Math.max(1, Math.round(meas.d / bq));
        for (let j = 0; j < n; j++) clicks.push({ s: sec + secBetween(meas.t, meas.t + j * bq), accent: j === 0 });
      }
      sec += dur;
    }
    const events = [];
    for (const line of lines) {
      for (const p of perf) {
        const meas = M.measures[p.mi];
        for (const e of (line.byM.get(p.mi) || [])) {
          events.push({ s: p.sec + secBetween(meas.t, e.t), d: secBetween(e.t, e.t + e.d), midi: e.m, line });
        }
      }
    }
    events.sort((a, b) => a.s - b.s);
    clicks.sort((a, b) => a.s - b.s);
    const words = [];
    const verse = M.verses[verseIdx];
    if (verse) {
      for (const p of perf) {
        const meas = M.measures[p.mi];
        for (const w of verse.words) if (w.mi === p.mi) words.push({ s: p.sec + secBetween(meas.t, w.t), text: w.text });
      }
      words.sort((a, b) => a.s - b.s);
    }
    return { perf, events, clicks, words, total: sec, countInSec, range };
  }

  // ---------- audio player ----------
  const P = {
    ctx: null, master: null, clickGain: null, lineGain: new Map(),
    playing: false, plan: null, t0: 0, idx: 0, cidx: 0, timer: 0, uiTimer: 0,
    solo: null, lastK: -1, lastWord: -1, resume: null, onEnd: null,
  };

  function ensureCtx() {
    if (!P.ctx) {
      const AC = window.AudioContext || window.webkitAudioContext;
      if (!AC) throw new Error('This browser cannot play sound. Please use a current version of Edge, Chrome or Firefox.');
      P.ctx = new AC();
      const comp = P.ctx.createDynamicsCompressor();
      comp.threshold.value = -12; comp.ratio.value = 4;
      P.master = P.ctx.createGain(); P.master.gain.value = 0.8;
      P.master.connect(comp).connect(P.ctx.destination);
    }
    if (P.ctx.state === 'suspended') P.ctx.resume();
    if (!P.clickGain) { P.clickGain = P.ctx.createGain(); P.clickGain.gain.value = 0.7; P.clickGain.connect(P.master); }
  }
  function isOn(line) { return P.solo ? P.solo === line : line.enabled; }
  function gainFor(line) { return isOn(line) ? Math.pow(line.volume / 100, 2) : 0; }
  function lineNode(line) {
    let g = P.lineGain.get(line.id);
    if (!g) { g = P.ctx.createGain(); g.gain.value = gainFor(line); g.connect(P.master); P.lineGain.set(line.id, g); }
    return g;
  }
  function refreshGains() {
    if (!P.ctx) return;
    for (const l of lines) {
      const g = P.lineGain.get(l.id);
      if (g) g.gain.setTargetAtTime(gainFor(l), P.ctx.currentTime, 0.02);
    }
  }
  function silence() {
    clearInterval(P.timer); clearInterval(P.uiTimer);
    for (const g of P.lineGain.values()) { try { g.disconnect(); } catch (e) { /* ignore */ } }
    P.lineGain.clear();
    if (P.clickGain) { try { P.clickGain.disconnect(); } catch (e) { /* ignore */ } P.clickGain = null; }
    P.playing = false;
  }

  function startPlan(plan, fromSec, solo) {
    ensureCtx();
    silence();
    P.solo = solo || null;
    if (!P.clickGain) { P.clickGain = P.ctx.createGain(); P.clickGain.gain.value = 0.7; P.clickGain.connect(P.master); }
    P.plan = plan;
    P.t0 = P.ctx.currentTime + 0.12 - fromSec;
    P.idx = plan.events.findIndex((e) => e.s >= fromSec - 0.001); if (P.idx < 0) P.idx = plan.events.length;
    P.cidx = plan.clicks.findIndex((c) => c.s >= fromSec - 0.001); if (P.cidx < 0) P.cidx = plan.clicks.length;
    P.lastK = -1; P.lastWord = -1;
    P.playing = true;
    P.timer = setInterval(scheduleAhead, 25);
    P.uiTimer = setInterval(updatePosition, 120);
    scheduleAhead();
    updatePlayButton();
  }

  function scheduleAhead() {
    const ctx = P.ctx, now = ctx.currentTime, horizon = now + 0.25;
    const ev = P.plan.events;
    while (P.idx < ev.length && P.t0 + ev[P.idx].s < horizon) {
      const e = ev[P.idx++];
      if (!isOn(e.line)) continue;
      const when = Math.max(P.t0 + e.s, now + 0.005);
      Synth.note(ctx, lineNode(e.line), e.line.sound, e.midi + e.line.shift, when, e.d, 0.9);
    }
    const cl = P.plan.clicks;
    while (P.cidx < cl.length && P.t0 + cl[P.cidx].s < horizon) {
      const c = cl[P.cidx++];
      Synth.click(ctx, P.clickGain, Math.max(P.t0 + c.s, now + 0.005), c.accent);
    }
    if (now > P.t0 + P.plan.total + 0.35) finished();
  }

  function playPosition() {
    // returns {k, frac} of the current spot in the plan
    const now = P.ctx.currentTime - P.t0;
    const perf = P.plan.perf;
    let k = 0;
    while (k + 1 < perf.length && perf[k + 1].sec <= now) k++;
    const frac = perf[k].dur > 0 ? Math.max(0, Math.min(1, (now - perf[k].sec) / perf[k].dur)) : 0;
    return { k, frac, now };
  }

  function updatePosition() {
    if (!P.playing) return;
    const { k, now } = playPosition();
    if (now < P.plan.countInSec) {
      if (P.lastK !== -2) { P.lastK = -2; $('position-text').textContent = 'Counting in.'; }
      return;
    }
    if (k !== P.lastK) {
      P.lastK = k;
      const mi = P.plan.perf[k].mi;
      const meas = M.measures[mi];
      $('position-text').textContent = `Playing measure ${meas.n} of ${lastNumber()}.`;
      markCurrent(mi);
      if (opts.sayMeasures) announce(`Measure ${meas.n}`);
    }
    const w = P.plan.words;
    let wi = P.lastWord;
    while (wi + 1 < w.length && w[wi + 1].s <= now) wi++;
    if (wi !== P.lastWord && wi >= 0) {
      P.lastWord = wi;
      $('lyric-now').textContent = `Words: ${w[wi].text}`;
      if (opts.sayLyrics) announce(w[wi].text);
    }
  }

  function markCurrent(mi) {
    for (const li of document.querySelectorAll('#lyrics li[data-from]')) {
      const cur = mi >= +li.dataset.from && mi <= +li.dataset.to;
      if (cur) li.setAttribute('aria-current', 'true'); else li.removeAttribute('aria-current');
    }
    for (const tr of document.querySelectorAll('#notes-wrap tr[data-mi]')) {
      if (+tr.dataset.mi === mi) tr.setAttribute('aria-current', 'true'); else tr.removeAttribute('aria-current');
    }
  }

  function finished() {
    const onEnd = P.onEnd; P.onEnd = null;
    silence();
    if (onEnd) { onEnd(); return; }
    if (opts.loop && !P.solo) {
      startPlan(buildPlan(currentRange(), false, opts.metronome, opts.verse), -0.4, null);
      return;
    }
    P.resume = null;
    $('position-text').textContent = 'Finished. Press Play to start again.';
    markCurrent(-1);
    updatePlayButton();
    announce('Finished.');
  }

  function play() {
    setError('');
    if (!lines.some((l) => l.enabled)) {
      setError('No lines are chosen. Check at least one line under Lines to play.');
      return;
    }
    try { ensureCtx(); } catch (e) { setError(e.message); return; }
    const range = currentRange();
    if (P.resume && P.resume.from === range.from && P.resume.to === range.to) {
      const plan = buildPlan(range, false, opts.metronome, opts.verse);
      const p = plan.perf[Math.min(P.resume.k, plan.perf.length - 1)];
      P.resume = null;
      startPlan(plan, p.sec + P.resume_frac * p.dur, null);
      return;
    }
    P.resume = null;
    startPlan(buildPlan(range, opts.countIn, opts.metronome, opts.verse), 0, null);
    announce(`Playing measures ${M.measures[range.from].n} to ${M.measures[range.to].n}.`);
  }

  function pause() {
    if (!P.playing) return;
    const { k, frac } = playPosition();
    const wasSolo = P.solo;
    silence();
    if (!wasSolo) {
      P.resume = { k: Math.max(0, k), from: P.plan.range.from, to: P.plan.range.to };
      P.resume_frac = frac;
      const meas = M.measures[P.plan.perf[k].mi];
      $('position-text').textContent = `Paused in measure ${meas.n}. Press Play to continue.`;
    }
    updatePlayButton();
    announce('Paused.');
  }

  function stop() {
    const was = P.playing || P.resume;
    silence();
    P.resume = null; P.onEnd = null;
    $('position-text').textContent = 'Stopped. Play starts from the "From measure" box.';
    $('lyric-now').textContent = '';
    markCurrent(-1);
    updatePlayButton();
    if (was) announce('Stopped.');
  }

  // restart at the same spot (used when the tempo changes during playback)
  function restartHere() {
    if (!P.playing || P.solo) return;
    const { k, frac } = playPosition();
    const plan = buildPlan(P.plan.range, false, opts.metronome, opts.verse);
    const p = plan.perf[Math.min(k, plan.perf.length - 1)];
    startPlan(plan, p.sec + frac * p.dur, null);
  }

  function updatePlayButton() {
    const b = $('btn-play');
    b.textContent = P.playing && !P.solo ? 'Pause' : (P.resume ? 'Continue' : 'Play');
  }

  function playMeasureOf(line, mi) {
    try { ensureCtx(); } catch (e) { setError(e.message); return; }
    P.resume = null;
    const savedLines = lines;
    const plan = buildPlan({ from: mi, to: mi }, false, false, opts.verse);
    plan.events = plan.events.filter((e) => e.line === line);
    lines = savedLines;
    if (!plan.events.length) { announce(`Measure ${M.measures[mi].n} is a rest for this line.`); return; }
    P.onEnd = () => { $('position-text').textContent = `Played measure ${M.measures[mi].n}.`; markCurrent(-1); updatePlayButton(); };
    startPlan(plan, 0, line);
  }

  function startingNotes() {
    setError('');
    const chosen = lines.filter((l) => l.enabled);
    if (!chosen.length) { setError('No lines are chosen. Check at least one line under Lines to play.'); return; }
    try { ensureCtx(); } catch (e) { setError(e.message); return; }
    stop();
    const range = currentRange();
    const firsts = [];
    for (const l of chosen) {
      let found = null;
      for (let mi = range.from; mi < M.measures.length && !found; mi++) {
        const evs = l.byM.get(mi);
        if (evs && evs.length) found = evs[0];
      }
      if (found) firsts.push({ l, e: found });
    }
    if (!firsts.length) { setError('The chosen lines have no notes after that measure.'); return; }
    firsts.sort((a, b) => a.e.m - b.e.m);
    const t = P.ctx.currentTime + 0.1;
    const dest = P.master;
    firsts.sort((a, b) => (a.e.m + a.l.shift) - (b.e.m + b.l.shift));
    firsts.forEach((f, i) => Synth.note(P.ctx, dest, f.l.sound, f.e.m + f.l.shift, t + i * 1.1, 0.95, 0.8));
    firsts.forEach((f) => Synth.note(P.ctx, dest, f.l.sound, f.e.m + f.l.shift, t + firsts.length * 1.1, 1.8, 0.6));
    const desc = firsts.map((f) => `${lineName(f.l)}: ${noteName(f.e, f.l.shift)}`).join('; ');
    setStatus(`Starting notes, lowest first, then together. ${desc}.`);
  }

  const PC_NAMES = ['C', 'C sharp', 'D', 'E flat', 'E', 'F', 'F sharp', 'G', 'A flat', 'A', 'B flat', 'B'];
  // the note's name as printed, or its sounding pitch when the line is shifted
  function noteName(e, shift) {
    if (!shift) return e.name;
    const m = e.m + shift;
    return `${PC_NAMES[((m % 12) + 12) % 12]} ${Math.floor(m / 12) - 1}`;
  }

  // ---------- saving audio ----------
  async function saveWav() {
    setError('');
    const chosen = lines.filter((l) => l.enabled);
    if (!chosen.length) { setError('No lines are chosen. Check at least one line under Lines to play.'); return; }
    const OAC = window.OfflineAudioContext || window.webkitOfflineAudioContext;
    if (!OAC) { setError('This browser cannot make audio files.'); return; }
    const plan = buildPlan(currentRange(), false, opts.metronome, opts.verse);
    const rate = 32000;
    const ctx = new OAC(1, Math.ceil((plan.total + 1.5) * rate), rate);
    const comp = ctx.createDynamicsCompressor(); comp.threshold.value = -12; comp.ratio.value = 4;
    const master = ctx.createGain(); master.gain.value = 0.8; master.connect(comp).connect(ctx.destination);
    const gains = new Map();
    for (const l of chosen) { const g = ctx.createGain(); g.gain.value = Math.pow(l.volume / 100, 2); g.connect(master); gains.set(l, g); }
    const btn = $('btn-wav');
    btn.disabled = true;
    setStatus('Making the audio file. This can take a little while for a long piece.');
    try {
      for (const e of plan.events) if (gains.has(e.line)) Synth.note(ctx, gains.get(e.line), e.line.sound, e.midi + e.line.shift, e.s + 0.2, e.d, 0.9);
      for (const c of plan.clicks) Synth.click(ctx, master, c.s + 0.2, c.accent);
      const buf = await ctx.startRendering();
      const blob = Synth.toWav(buf);
      const r = plan.range;
      const what = chosen.length === 1 ? lineName(chosen[0]) : `${chosen.length} lines`;
      const name = `${score.meta.title} - ${what} - m${M.measures[r.from].n}-${M.measures[r.to].n} - ${opts.tempo}bpm.wav`
        .replace(/[\\/:*?"<>|()]+/g, '').replace(/\s+/g, ' ');
      const a = el('a', { href: URL.createObjectURL(blob), download: name });
      document.body.append(a); a.click(); a.remove();
      setTimeout(() => URL.revokeObjectURL(a.href), 60000);
      setStatus(`Your browser has saved the audio file "${name}", usually in your Downloads folder.`);
    } catch (e) {
      setError('The audio file could not be made: ' + e.message);
    } finally {
      btn.disabled = false;
    }
  }

  // ---------- preferences ----------
  let saveTimer;
  function savePrefs() {
    if (!score) return;
    clearTimeout(saveTimer);
    saveTimer = setTimeout(() => {
      const p = { opts: Object.assign({}, opts), lines: {} };
      for (const l of lines) p.lines[l.id] = { enabled: l.enabled, volume: l.volume, sound: l.sound, shift: l.shift, label: l.label };
      api(`/api/scores/${score.id}/prefs`, { method: 'PUT', body: JSON.stringify(p), headers: { 'Content-Type': 'application/json' } }).catch(() => {});
    }, 500);
  }

  // ---------- rendering the score page ----------
  function loadScore(data) {
    stop();
    score = data;
    M = data.model;
    M.verses = M.verses || [];
    M.warnings = M.warnings || [];
    M.lines = M.lines || [];
    const prefs = data.prefs || {};
    const lp = prefs.lines || {};
    lines = M.lines.map((l) => {
      const p = lp[l.id] || {};
      const byM = new Map();
      for (const e of l.events) { if (!byM.has(e.mi)) byM.set(e.mi, []); byM.get(e.mi).push(e); }
      return Object.assign({}, l, {
        byM,
        enabled: p.enabled != null ? p.enabled : l.default,
        volume: p.volume != null ? p.volume : ({ voice: 80, instrument: 75, other: 75, keyboard: 55 }[l.kind] || 70),
        sound: p.sound || 'piano',
        shift: p.shift != null ? p.shift : (p.octave ? p.octave * 12 : (l.transpose || 0)),
        label: p.label || '',
      });
    });
    const first = M.measures[0];
    beatQn0 = beatQn(first);
    const scoreTempo = Math.round(M.tempo[0].bpm / beatQn0);
    Object.assign(opts, {
      tempo: scoreTempo, from: first.n, to: lastNumber(), loop: false, countIn: true, metronome: false,
      repeats: true, sayMeasures: false, sayLyrics: false, verse: 0, notesLine: '',
    }, prefs.opts || {});
    if (opts.verse >= M.verses.length) opts.verse = 0;
    factor = (opts.tempo * beatQn0) / M.tempo[0].bpm;
    P.resume = null;

    $('score-title').textContent = score.meta.title;
    $('rename').value = score.meta.title;
    const words = M.verses.reduce((n, v) => n + v.words.length, 0);
    $('score-summary').textContent = `${M.measures.length} measures, ${describeParts()}` +
      (words ? `, lyrics found.` : `, no lyrics found.`) +
      ` Notes and lyrics were read automatically, so check anything that sounds wrong against the printed score.`;
    const wl = $('score-warnings'); wl.textContent = '';
    for (const w of M.warnings || []) wl.append(el('li', { text: w }));

    $('tempo').value = opts.tempo;
    $('tempo-help').textContent = `One beat is a ${beatName(beatQn0)}. ` + (M.tempoFromScore
      ? `The score asks for ${scoreTempo} beats per minute.`
      : `The score has no tempo marking that could be read, so ${Math.round(90 / beatQn0)} is used to start with.`);
    for (const id of ['from', 'to']) { $(id).min = first.n; $(id).max = lastNumber(); }
    $('from').value = opts.from; $('to').value = opts.to;
    $('loop').checked = opts.loop; $('countin').checked = opts.countIn; $('metronome').checked = opts.metronome;
    $('repeats').checked = opts.repeats; $('repeats-wrap').hidden = !M.hasRepeats;
    $('say-measures').checked = opts.sayMeasures; $('say-lyrics').checked = opts.sayLyrics;
    $('btn-reread').hidden = score.meta.kind !== 'pdf';
    $('position-text').textContent = 'Stopped at the beginning.';
    $('lyric-now').textContent = '';

    renderLines();
    renderLyrics();
    renderNotesSelect();
    renderNotes();
    updatePlayButton();
    $('player').hidden = false;
    document.title = `${score.meta.title} - score2audio`;
  }

  const GROUPS = [['voice', 'Voices', 'voices'], ['instrument', 'Instruments', 'instruments'],
    ['other', 'Other lines', 'other lines'], ['keyboard', 'Keyboards', 'keyboard lines']];

  function describeParts() {
    const parts = [];
    for (const [kind, , plural] of GROUPS) {
      const n = lines.filter((l) => l.kind === kind && l.primary).length;
      if (n) parts.push(`${n} ${n === 1 ? plural.replace(/s$/, '').replace('lines', 'line') : plural}`);
    }
    return parts.join(', ') || 'no lines';
  }

  function renderLines() {
    const box = $('lines');
    box.textContent = '';
    const multiple = GROUPS.filter(([kind]) => lines.some((l) => l.kind === kind)).length > 1;
    for (const [kind, heading, plural] of GROUPS) {
      const group = lines.filter((l) => l.kind === kind);
      if (!group.length) continue;
      const fs = el('fieldset', { class: 'linegroup' }, el('legend', { text: heading }));
      if (multiple) {
        const only = el('button', { type: 'button', text: `Choose only the ${plural}` });
        only.addEventListener('click', () => {
          for (const l of lines) l.enabled = l.kind === kind && l.primary;
          renderLines(); refreshGains(); savePrefs();
          setStatus(`Only the ${plural} are chosen.`);
        });
        fs.append(el('div', { class: 'btnrow' }, only));
      }
      for (const l of group) fs.append(renderLine(l));
      box.append(fs);
    }
  }

  function renderLine(l) {
    const id = l.id;
    const details = [l.hint, l.clef && !l.name.includes(l.clef) ? l.clef : '', l.low ? `notes from ${l.low} to ${l.high} as printed` : 'no notes'].filter(Boolean).join(', ') +
      (l.transposeNote ? `. ${l.transposeNote}` : '');
    const cb = el('input', { type: 'checkbox', id: `en-${id}` });
    cb.checked = l.enabled;
    cb.addEventListener('change', () => { l.enabled = cb.checked; refreshGains(); savePrefs(); });

    const vol = el('input', { type: 'range', id: `vol-${id}`, min: 0, max: 100, step: 5, value: l.volume });
    vol.setAttribute('aria-valuetext', `${l.volume} percent`);
    vol.addEventListener('input', () => {
      l.volume = +vol.value;
      vol.setAttribute('aria-valuetext', `${l.volume} percent`);
      refreshGains(); savePrefs();
    });

    const snd = el('select', { id: `snd-${id}` });
    for (const s of Synth.SOUNDS) { const o = el('option', { value: s.id, text: s.name }); if (s.id === l.sound) o.selected = true; snd.append(o); }
    snd.addEventListener('change', () => { l.sound = snd.value; savePrefs(); });

    const oct = el('select', { id: `oct-${id}` });
    const PITCH = [[0, 'As printed'], [-12, 'One octave lower'], [12, 'One octave higher'], [-24, 'Two octaves lower'], [24, 'Two octaves higher'],
      [-2, 'B-flat instrument: a whole step lower'], [-3, 'A instrument: a minor third lower'], [-5, 'G instrument: a fourth lower'],
      [-7, 'F instrument: a fifth lower'], [-9, 'E-flat alto instrument: a major sixth lower'], [3, 'E-flat instrument: a minor third higher'],
      [2, 'D instrument: a whole step higher'], [-14, 'B-flat tenor instrument: an octave and a whole step lower'],
      [-21, 'E-flat baritone instrument: an octave and a major sixth lower']];
    if (!PITCH.some(([v]) => v === l.shift)) PITCH.push([l.shift, `${l.shift > 0 ? 'Up' : 'Down'} ${Math.abs(l.shift)} half steps`]);
    for (const [v, t] of PITCH) {
      const o = el('option', { value: v, text: t + (v === (l.transpose || 0) && v !== 0 ? ' (suggested by the label)' : '') });
      if (v === l.shift) o.selected = true;
      oct.append(o);
    }
    oct.addEventListener('change', () => { l.shift = +oct.value; savePrefs(); renderNotes(); });

    const nameIn = el('input', { type: 'text', id: `nm-${id}`, value: l.label || '', autocomplete: 'off', placeholder: '' });
    const nameBtn = el('button', { type: 'button', text: 'Save name' });
    nameBtn.addEventListener('click', () => {
      l.label = nameIn.value.trim();
      savePrefs();
      renderLines(); renderNotesSelect(); renderNotes();
      const again = $(`nm-${id}`);
      if (again) { again.closest('details').open = true; again.focus(); }
      setStatus(l.label ? `Line renamed to ${l.label}.` : `Line name set back to ${l.name}.`);
    });

    const hidden = (t) => el('span', { class: 'visually-hidden', text: t });
    return el('div', { class: 'line' },
      el('div', { class: 'check' }, cb,
        el('label', { for: `en-${id}` },
          el('span', { class: 'name', text: lineName(l) }),
          l.label ? el('span', { class: 'detail' }, hidden(', '), `(${l.name})`) : null,
          el('span', { class: 'detail' }, hidden(', '), details))),
      el('div', { class: 'controls' },
        el('label', { for: `vol-${id}` }, 'Volume', hidden(` for ${lineName(l)}`)), vol),
      el('details', {},
        el('summary', {}, 'More settings', hidden(` for ${lineName(l)}`)),
        el('div', { class: 'field' }, el('label', { for: `snd-${id}` }, 'Sound', hidden(` for ${lineName(l)}`)), snd),
        el('div', { class: 'field' }, el('label', { for: `oct-${id}` }, 'Pitch', hidden(` for ${lineName(l)}`)), oct),
        el('div', { class: 'field' }, el('label', { for: `nm-${id}` }, 'Your name for this line, for example Bass or First violin', hidden(` (${l.name})`)), nameIn, nameBtn)));
  }

  function renderLyrics() {
    const box = $('lyrics');
    box.textContent = '';
    if (!M.verses.length) {
      box.append(el('p', { text: 'No lyrics were found in this score.' }));
      return;
    }
    box.append(el('p', { class: 'help', text: 'Lyrics are read from the printed page by text recognition, so some words may be missing or misspelled. Each line below matches one line of music on the page.' }));
    if (M.verses.length > 1) {
      const sel = el('select', { id: 'verse' });
      M.verses.forEach((v, i) => { const o = el('option', { value: i, text: v.name }); if (i === opts.verse) o.selected = true; sel.append(o); });
      sel.addEventListener('change', () => { opts.verse = +sel.value; savePrefs(); renderLyricLines(); });
      box.append(el('div', { class: 'field' }, el('label', { for: 'verse', text: 'Lyric line to show' }), sel));
    }
    box.append(el('ol', { class: 'lyric-lines', id: 'lyric-list' }));
    renderLyricLines();
  }

  function renderLyricLines() {
    const ol = $('lyric-list');
    ol.textContent = '';
    const v = M.verses[opts.verse];
    for (const ln of v.lines) {
      const a = M.measures[ln.from].n, b = M.measures[ln.to].n;
      const btn = el('button', { type: 'button', text: `Play from measure ${a}` });
      btn.addEventListener('click', () => {
        opts.from = a; $('from').value = a; savePrefs();
        stop(); play();
      });
      ol.append(el('li', { 'data-from': ln.from, 'data-to': ln.to },
        el('span', { class: 'mnum', text: a === b ? `Measure ${a}` : `Measures ${a} to ${b}` }),
        el('span', { class: 'lyric-text', text: ln.text }), ' ', btn));
    }
  }

  function renderNotesSelect() {
    const sel = $('notes-line');
    sel.textContent = '';
    if (!lines.find((l) => l.id === opts.notesLine)) {
      const pick = lines.find((l) => l.enabled) || lines[0];
      opts.notesLine = pick ? pick.id : '';
    }
    for (const l of lines) {
      const o = el('option', { value: l.id, text: lineName(l) });
      if (l.id === opts.notesLine) o.selected = true;
      sel.append(o);
    }
  }

  const QN = [[8, 'two whole notes'], [6, 'dotted whole'], [4, 'whole'], [3, 'dotted half'], [2, 'half'], [1.5, 'dotted quarter'],
    [1, 'quarter'], [0.75, 'dotted eighth'], [0.5, 'eighth'], [0.25, 'sixteenth'], [2 / 3, 'triplet quarter'], [1 / 3, 'triplet eighth']];
  function qnName(q) {
    for (const [v, n] of QN) if (Math.abs(q - v) < 0.02) return n;
    // split odd lengths into parts
    let rest = q; const parts = [];
    for (const [v, n] of QN) { while (rest >= v - 0.02 && parts.length < 4) { parts.push(n); rest -= v; } }
    return parts.length ? parts.join(' plus ') : `${Math.round(q * 100) / 100} beats`;
  }

  function measureText(line, mi) {
    const meas = M.measures[mi];
    const end = meas.t + meas.d;
    let cursor = meas.t;
    const parts = [];
    // a note held over from earlier measures
    for (let back = mi - 1; back >= 0 && back >= mi - 8; back--) {
      for (const e of line.byM.get(back) || []) {
        if (e.t + e.d > meas.t + 0.01) { cursor = Math.max(cursor, Math.min(end, e.t + e.d)); }
      }
    }
    if (cursor > meas.t + 0.01) parts.push('held note continues');
    const evs = line.byM.get(mi) || [];
    const groups = [];
    for (const e of evs) {
      const g = groups.find((x) => Math.abs(x.t - e.t) < 0.001);
      if (g) g.es.push(e); else groups.push({ t: e.t, es: [e] });
    }
    groups.sort((a, b) => a.t - b.t);
    for (const g of groups) {
      if (g.t > cursor + 0.02) parts.push(`${qnName(g.t - cursor)} rest`);
      const names = g.es.slice().sort((a, b) => a.m - b.m).map((e) => noteName(e, line.shift));
      const longest = g.es.reduce((a, b) => (b.d > a.d ? b : a));
      let txt = names.length > 1 ? `${names.join(' with ')}, ${longest.dur}` : `${names[0]}, ${longest.dur}`;
      if (longest.t + longest.d > end + 0.02) txt += ', held into the next measure';
      parts.push(txt);
      cursor = Math.max(cursor, longest.t + longest.d);
    }
    if (!groups.length && cursor <= meas.t + 0.01) return 'Rest for the whole measure';
    if (cursor < end - 0.02) parts.push(`${qnName(end - cursor)} rest`);
    return parts.join('; ');
  }

  function renderNotes() {
    const wrap = $('notes-wrap');
    wrap.textContent = '';
    const line = lines.find((l) => l.id === opts.notesLine);
    if (!line) return;
    const verse = M.verses[opts.verse];
    const tbody = el('tbody');
    M.measures.forEach((meas, mi) => {
      let words = '';
      const own = (line.byM.get(mi) || []).map((e) => e.ly).filter(Boolean);
      if (verse) words = verse.words.filter((w) => w.mi === mi).map((w) => w.text).join(' ');
      if (!words && own.length) words = own.join(' ');
      const btn = el('button', { type: 'button', text: 'Play', 'aria-label': `Play measure ${meas.n}` });
      btn.addEventListener('click', () => playMeasureOf(line, mi));
      tbody.append(el('tr', { 'data-mi': mi },
        el('th', { scope: 'row', text: meas.n }),
        el('td', { text: measureText(line, mi) }),
        el('td', { text: words }),
        el('td', {}, btn)));
    });
    wrap.append(el('table', {},
      el('caption', { text: `Notes for ${lineName(line)}` + (line.shift ? ', at sounding pitch' : '') }),
      el('thead', {}, el('tr', {},
        el('th', { scope: 'col', text: 'Measure' }), el('th', { scope: 'col', text: 'Notes' }),
        el('th', { scope: 'col', text: 'Words' }), el('th', { scope: 'col', text: 'Listen' }))),
      tbody));
  }

  // ---------- library, opening, setup ----------
  async function refreshLibrary() {
    const list = $('library');
    list.textContent = '';
    let items = [];
    try { items = await api('/api/library'); } catch (e) { setError(e.message); return; }
    $('library-empty').textContent = items.length
      ? 'Scores you have opened before. Each one remembers your line and tempo choices.'
      : 'Scores you open are kept here, so you only need to read each PDF once.';
    for (const it of items) {
      const date = new Date(it.created).toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric' });
      const open = el('button', { type: 'button', text: 'Open', 'aria-label': `Open ${it.title}` });
      open.addEventListener('click', () => openScore(it.id));
      const del = el('button', { type: 'button', text: 'Delete', 'aria-label': `Delete ${it.title}` });
      del.addEventListener('click', async () => {
        if (!confirm(`Delete "${it.title}" from your scores? The original file on your computer is not affected.`)) return;
        try {
          await api(`/api/scores/${it.id}`, { method: 'DELETE' });
          if (score && score.id === it.id) closeScore(false);
          setStatus(`Deleted ${it.title}.`);
          await refreshLibrary();
          $('h-library').setAttribute('tabindex', '-1'); $('h-library').focus();
        } catch (e) { setError(e.message); }
      });
      list.append(el('li', {}, el('strong', { text: it.title }),
        el('span', { class: 'meta', text: `Added ${date}, ${it.measures} measures, from ${it.fileName}` }), open, del));
    }
  }

  async function openScore(id) {
    setError('');
    try {
      const data = await api(`/api/scores/${id}`);
      loadScore(data);
      setStatus(`Opened ${score.meta.title}: ${M.measures.length} measures, ${describeParts()}.`);
      $('score-title').focus();
    } catch (e) { setError(e.message); }
  }

  function closeScore(focus = true) {
    stop();
    score = null; M = null; lines = [];
    $('player').hidden = true;
    document.title = 'score2audio';
    if (focus) { $('h-open').setAttribute('tabindex', '-1'); $('h-open').focus(); }
  }

  let polling = false;
  async function watchJob(jobId, scoreId) {
    polling = true;
    const box = $('job'), bar = $('job-progress'), txt = $('job-text');
    box.hidden = false;
    bar.removeAttribute('value');
    let lastPage = 0;
    setStatus('Reading the score. This page will tell you when it is ready.');
    try {
      for (;;) {
        await new Promise((r) => setTimeout(r, 1500));
        const j = await api(`/api/jobs/${jobId}`);
        if (j.pages > 0) { bar.max = j.pages; bar.value = Math.max(0, j.page - 1); }
        txt.textContent = `${j.message}. ${j.elapsed} seconds so far.`;
        if (j.page !== lastPage && j.pages > 0 && j.state === 'running') {
          lastPage = j.page;
          setStatus(`${j.message}.`);
        }
        if (j.state === 'done') {
          bar.value = bar.max;
          txt.textContent = `Finished in ${j.elapsed} seconds.`;
          await refreshLibrary();
          await openScore(scoreId);
          break;
        }
        if (j.state === 'error') {
          txt.textContent = '';
          setError(j.message);
          break;
        }
      }
    } catch (e) { setError(e.message); }
    box.hidden = true;
    polling = false;
  }

  async function submitOpen(ev) {
    ev.preventDefault();
    setError('');
    const input = $('file');
    const file = input.files && input.files[0];
    if (!file) { setError('Choose a score file first.'); input.focus(); return; }
    if (polling) { setError('A score is already being read. Please wait for it to finish.'); return; }
    try {
      setStatus(`Sending ${file.name}.`);
      const r = await api(`/api/upload?name=${encodeURIComponent(file.name)}`, { method: 'POST', body: file });
      input.value = '';
      if (r.jobId) await watchJob(r.jobId, r.scoreId);
      else { await refreshLibrary(); await openScore(r.scoreId); }
    } catch (e) { setError(e.message); }
  }

  async function refreshEngine() {
    try {
      const s = await api('/api/status');
      $('version').textContent = 'version ' + s.version;
      for (const li of document.querySelectorAll('[data-os]')) li.hidden = li.dataset.os !== s.os && !(li.dataset.os === 'linux' && !['windows', 'darwin'].includes(s.os));
      $('engine-path-help').textContent = { windows: 'For example: C:\\Program Files\\Audiveris', darwin: 'For example: /Applications/Audiveris.app' }[s.os] || 'For example: /opt/audiveris/bin/Audiveris';
      if (s.audiveris) {
        $('engine-status').textContent = `Ready. The music reading program Audiveris was found at ${s.audiveris}.`;
        $('engine-help').hidden = true;
      } else {
        $('engine-status').textContent = 'The music reading program Audiveris was not found, so PDF files cannot be read yet.';
        $('engine-help').hidden = false;
      }
      $('engine-path').value = s.savedAudiveris || '';
      $('ocr-status').textContent = s.tessdata
        ? 'Lyrics reading is available.'
        : 'Lyrics reading data was not found, so lyrics will not be read from PDFs. Keep the tessdata folder next to the score2audio program.';
      return s;
    } catch (e) { setError(e.message); return null; }
  }

  // ---------- wiring ----------
  function numberInput(id, key, after) {
    $(id).addEventListener('change', () => {
      const v = $(id).value.trim();
      if (key === 'tempo') {
        const n = Math.round(+v);
        if (!(n >= 20 && n <= 300)) { setError('Tempo must be a number from 20 to 300.'); $(id).value = opts.tempo; return; }
        opts.tempo = n;
      } else {
        if (measureIndex(v) < 0 || v === '') { setError(`Enter a measure number from ${M.measures[0].n} to ${lastNumber()}.`); $(id).value = opts[key]; return; }
        opts[key] = M.measures[measureIndex(v)].n;
        $(id).value = opts[key];
        P.resume = null; updatePlayButton();
      }
      setError('');
      savePrefs();
      if (after) after();
    });
  }
  function checkbox(id, key, after) {
    $(id).addEventListener('change', () => { opts[key] = $(id).checked; savePrefs(); if (after) after(); });
  }

  $('open-form').addEventListener('submit', submitOpen);
  $('btn-play').addEventListener('click', () => (P.playing && !P.solo ? pause() : play()));
  $('btn-stop').addEventListener('click', stop);
  $('btn-pitches').addEventListener('click', startingNotes);
  numberInput('tempo', 'tempo', () => {
    factor = (opts.tempo * beatQn0) / M.tempo[0].bpm;
    restartHere();
    setStatus(`Tempo set to ${opts.tempo} beats per minute.`);
  });
  numberInput('from', 'from');
  numberInput('to', 'to');
  checkbox('loop', 'loop');
  checkbox('countin', 'countIn');
  checkbox('metronome', 'metronome', restartHere);
  checkbox('repeats', 'repeats', () => { P.resume = null; updatePlayButton(); });
  checkbox('say-measures', 'sayMeasures');
  checkbox('say-lyrics', 'sayLyrics');
  $('btn-every').addEventListener('click', () => {
    for (const l of lines) l.enabled = l.primary;
    renderLines(); refreshGains(); savePrefs();
    setStatus('Every line chosen. Staves holding two parts play their upper and lower notes, or all notes for instruments and keyboards.');
  });
  $('btn-none').addEventListener('click', () => {
    for (const l of lines) l.enabled = false;
    renderLines(); refreshGains(); savePrefs();
    setStatus('No lines chosen.');
  });
  $('notes-line').addEventListener('change', () => { opts.notesLine = $('notes-line').value; savePrefs(); renderNotes(); });
  $('btn-wav').addEventListener('click', saveWav);
  $('btn-xml').addEventListener('click', async () => {
    try {
      const r = await fetch(`/api/scores/${score.id}/musicxml`, { headers: { 'X-Token': TOKEN } });
      if (!r.ok) throw new Error(await r.text());
      const cd = r.headers.get('content-disposition') || '';
      const name = (cd.match(/filename="([^"]+)"/) || [])[1] || 'score.mxl';
      const a = el('a', { href: URL.createObjectURL(await r.blob()), download: name });
      document.body.append(a); a.click(); a.remove();
      setStatus(`Your browser has saved "${name}". You can open it in MuseScore or other music programs.`);
    } catch (e) { setError(e.message); }
  });
  $('btn-reread').addEventListener('click', async () => {
    if (polling) { setError('A score is already being read. Please wait for it to finish.'); return; }
    try {
      const id = score.id;
      const r = await api(`/api/scores/${id}/reread`, { method: 'POST' });
      closeScore(false);
      $('h-open').setAttribute('tabindex', '-1'); $('h-open').focus();
      await watchJob(r.jobId, id);
    } catch (e) { setError(e.message); }
  });
  $('btn-close').addEventListener('click', () => closeScore(true));
  $('btn-rename').addEventListener('click', async () => {
    const t = $('rename').value.trim();
    if (!t) { setError('The title cannot be empty.'); $('rename').focus(); return; }
    try {
      const meta = await api(`/api/scores/${score.id}/rename`, { method: 'POST', body: JSON.stringify({ title: t }), headers: { 'Content-Type': 'application/json' } });
      score.meta = meta;
      $('score-title').textContent = meta.title;
      document.title = `${meta.title} - score2audio`;
      setStatus(`Title saved: ${meta.title}.`);
      refreshLibrary();
    } catch (e) { setError(e.message); }
  });
  $('engine-form').addEventListener('submit', async (ev) => {
    ev.preventDefault();
    setError('');
    try {
      await api('/api/settings', { method: 'POST', body: JSON.stringify({ audiverisPath: $('engine-path').value }), headers: { 'Content-Type': 'application/json' } });
      const s = await refreshEngine();
      setStatus(s && s.audiveris ? 'Location saved. Audiveris is ready.' : 'Location saved.');
    } catch (e) { setError(e.message); $('engine-path').focus(); }
  });
  $('btn-recheck').addEventListener('click', async () => {
    const s = await refreshEngine();
    if (s) setStatus(s.audiveris ? 'Audiveris was found. PDF scores can be read.' : 'Audiveris was still not found.');
  });
  $('btn-quit').addEventListener('click', async () => {
    if (!confirm('Quit score2audio? You can start it again at any time.')) return;
    stop();
    try { await api('/api/quit', { method: 'POST' }); } catch (e) { /* already closed */ }
    document.querySelector('main').hidden = true;
    $('btn-quit').hidden = true;
    setStatus('score2audio has been closed. You can close this browser tab.');
  });

  // ---------- start ----------
  (async () => {
    if (!TOKEN) {
      setError('This page was not opened by score2audio. Start score2audio, which opens the right page for you.');
      return;
    }
    await refreshEngine();
    await refreshLibrary();
  })();

  // for automated tests
  window.__score2audio = { get model() { return M; }, get lines() { return lines; }, opts, buildPlan: (r) => buildPlan(r || currentRange(), false, false, opts.verse) };
})();
