'use strict';
// Makes an MP4 video of the chosen lines: the music engraved by Verovio,
// scrolling with the current measure highlighted, the lyrics underneath,
// and the same sound as "Save as audio file". Everything happens in the
// browser (WebCodecs), so nothing else needs installing.

const VideoMaker = (() => {
  const W = 1280, H = 720, FPS = 30;
  const HEADER = 64, LYRICS = 124, PROGRESS = 6;
  const MUSIC_TOP = HEADER, MUSIC_H = H - HEADER - LYRICS;
  const LEAD = 1.0, TAIL = 2.0;      // seconds of picture before and after the music
  const RATE = 48000;
  const SCROLL = 0.6;                // seconds a scroll to the next line takes
  const PAD = 10;                    // pixels kept above and below each line of music

  class Cancelled extends Error {}

  // ---------- what this browser can do ----------

  async function codecs() {
    if (!('VideoEncoder' in window) || !('AudioEncoder' in window) || !('VideoFrame' in window)) return null;
    let video = null;
    for (const codec of ['avc1.4d0028', 'avc1.42001f', 'avc1.640028']) {
      const c = { codec, width: W, height: H, bitrate: 2500000, framerate: FPS, avc: { format: 'avc' } };
      try { if ((await VideoEncoder.isConfigSupported(c)).supported) { video = c; break; } } catch (e) { /* try the next */ }
    }
    // AAC plays everywhere; Opus is the fallback for browsers without an AAC encoder
    let audio = null, audioCodec = null;
    for (const [codec, mux] of [['mp4a.40.2', 'aac'], ['opus', 'opus']]) {
      const c = { codec, sampleRate: RATE, numberOfChannels: 1, bitrate: 128000 };
      try { if ((await AudioEncoder.isConfigSupported(c)).supported) { audio = c; audioCodec = mux; break; } } catch (e) { /* try the next */ }
    }
    return video && audio ? { video, audio, audioCodec } : null;
  }

  // ---------- engraving ----------

  let verovioReady = null;
  function loadVerovio() {
    if (verovioReady) return verovioReady;
    verovioReady = new Promise((resolve, reject) => {
      const fail = () => {
        verovioReady = null;
        reject(new Error('The music engraver (Verovio) was not found. Keep the verovio folder next to the score2audio program.'));
      };
      const s = document.createElement('script');
      s.src = '/vendor/verovio-toolkit-wasm.js';
      s.onerror = fail;
      s.onload = () => {
        if (!window.verovio || !verovio.module) { fail(); return; }
        if (verovio.module.calledRun) resolve();
        else verovio.module.onRuntimeInitialized = () => resolve();
      };
      document.head.append(s);
    });
    return verovioReady;
  }

  // Places each page's SVG in a hidden box to find where its lines of music
  // (systems) and measures are, in pixels.
  function measurePages(svgs) {
    const box = document.createElement('div');
    box.setAttribute('aria-hidden', 'true');
    box.style.cssText = 'position:absolute;left:-30000px;top:0;visibility:hidden;';
    document.body.append(box);
    const systems = [], measures = [], sizes = [];
    try {
      svgs.forEach((svg, page) => {
        box.innerHTML = svg;
        const root = box.querySelector('svg');
        const rr = root.getBoundingClientRect();
        sizes.push({ w: Math.ceil(rr.width), h: Math.ceil(rr.height) });
        for (const sys of root.querySelectorAll('g.system')) {
          const r = sys.getBoundingClientRect();
          const index = systems.length;
          systems.push({ page, y: Math.max(0, r.top - rr.top - PAD), h: Math.min(rr.height, r.height + 2 * PAD) });
          for (const m of sys.querySelectorAll('g.measure')) {
            const mr = m.getBoundingClientRect();
            measures.push({ sys: index, x: mr.left - rr.left, w: mr.width });
          }
        }
      });
    } finally {
      box.remove();
    }
    return { systems, measures, sizes };
  }

  async function engrave(xml, staves) {
    await loadVerovio();
    const tk = new verovio.toolkit();
    const usable = W - 60;
    let scale = staves <= 1 ? 54 : staves === 2 ? 48 : staves <= 4 ? 42 : staves <= 6 ? 36 : 30;
    // one or two staves leave room to see the next line coming; with more,
    // a line may fill most of the screen, so the music stays readable
    const fill = staves <= 2 ? 0.6 : 0.9;
    for (let attempt = 0; ; attempt++) {
      tk.setOptions({
        scale,
        pageWidth: Math.round(usable * 100 / scale),
        pageHeight: Math.round(12000 * 100 / scale),   // pages of at most 12000 pixels
        adjustPageHeight: true,
        breaks: 'auto',
        header: 'none',
        footer: 'none',
        pageMarginTop: 30, pageMarginBottom: 30, pageMarginLeft: 30, pageMarginRight: 30,
        spacingSystem: 14,
      });
      if (!tk.loadData(xml)) throw new Error('The notation could not be engraved.');
      const svgs = [];
      for (let p = 1; p <= tk.getPageCount(); p++) svgs.push(tk.renderToSVG(p));
      const layout = measurePages(svgs);
      const tallest = Math.max(0, ...layout.systems.map((s) => s.h));
      // a line of music should leave room for the next one below it
      if (tallest <= MUSIC_H * fill || scale <= 22 || attempt >= 3) {
        layout.pages = await Promise.all(svgs.map((svg, i) => rasterize(svg, layout.sizes[i])));
        return layout;
      }
      scale = Math.max(22, Math.floor(scale * MUSIC_H * fill / tallest));
    }
  }

  async function rasterize(svg, size) {
    const url = URL.createObjectURL(new Blob([svg], { type: 'image/svg+xml' }));
    try {
      const img = new Image();
      img.src = url;
      await img.decode();
      const c = document.createElement('canvas');
      c.width = size.w;
      c.height = size.h;
      const g = c.getContext('2d', { alpha: false });
      g.fillStyle = '#fff';
      g.fillRect(0, 0, c.width, c.height);
      g.drawImage(img, 0, 0, size.w, size.h);
      return c;
    } finally {
      URL.revokeObjectURL(url);
    }
  }

  // ---------- drawing frames ----------

  function ease(x) { return x <= 0 ? 0 : x >= 1 ? 1 : x * x * (3 - 2 * x); }

  function fitText(g, text, maxW, size, min, weight) {
    let s = size;
    g.font = `${weight} ${s}px "Segoe UI", system-ui, Arial, sans-serif`;
    while (s > min && g.measureText(text).width > maxW) {
      s -= 1;
      g.font = `${weight} ${s}px "Segoe UI", system-ui, Arial, sans-serif`;
    }
    return s;
  }

  class Frames {
    constructor(job, layout) {
      this.job = job;
      this.layout = layout;
      const { systems, measures } = layout;
      this.canvas = document.createElement('canvas');
      this.canvas.width = W;
      this.canvas.height = H;
      this.g = this.canvas.getContext('2d', { alpha: false });
      this.x0 = Math.round((W - Math.max(...layout.sizes.map((s) => s.w))) / 2);
      // every line of music stacked in one tall column
      this.sysY = [];
      let y = 0;
      for (const s of systems) { this.sysY.push(y); y += s.h; }
      this.columnH = y;
      const perf = job.plan.perf;
      this.perfSys = perf.map((p) => {
        const m = measures[p.mi - job.from];
        return m ? m.sys : -1;
      });
      this.lastKey = '';
      this.k = 0;            // where perfAt last found the music
      this.lyrK = -1;        // the measure the lyric line was worked out for
      this.lyr = null;
      // the title bar's text never changes: fit it once
      const g = this.g;
      g.font = '600 22px "Segoe UI", system-ui, Arial, sans-serif';
      const rightW = g.measureText(`Measure ${job.measures[job.measures.length - 1].n}`).width + 40;
      this.titleSize = fitText(g, job.title, W - rightW - 24, 28, 18, 700);
      this.partsSize = fitText(g, job.partsText, W - rightW - 24, 18, 13, 400);
    }

    target(sys) {
      if (sys < 0) return 0;
      // the playing line at the top; near the end, stop once the last line is in view
      return Math.max(0, Math.min(this.sysY[sys], Math.max(0, this.columnH - MUSIC_H + 16)));
    }

    // frames go forward in time, so carry on from the last measure found
    perfAt(a) {
      const perf = this.job.plan.perf;
      let k = this.k;
      if (k >= perf.length || perf[k].sec > a) k = 0;
      while (k + 1 < perf.length && perf[k + 1].sec <= a) k++;
      this.k = k;
      return k;
    }

    scrollAt(a, k) {
      const perf = this.job.plan.perf;
      const here = this.target(this.perfSys[k]);
      const next = k + 1 < perf.length ? k + 1 : -1;
      if (next >= 0 && this.perfSys[next] !== this.perfSys[k]) {
        const start = perf[next].sec - SCROLL;
        if (a > start) return here + (this.target(this.perfSys[next]) - here) * ease((a - start) / SCROLL);
      }
      return here;
    }

    // the printed lyric line for the measure being played, and its words with times
    lyricsAt(a, k) {
      const { plan, verse } = this.job;
      if (!verse || !plan.words.length) return null;
      const perf = plan.perf;
      const mi = perf[k].mi;
      let li = verse.lines.findIndex((l) => mi >= l.from && mi <= l.to);
      if (li < 0) {
        // between sung lines: show the next one coming up
        li = verse.lines.findIndex((l) => l.from > mi);
        if (li < 0) return null;
        const words = plan.words.filter((w) => w.k > k && w.mi >= verse.lines[li].from && w.mi <= verse.lines[li].to);
        return { line: li, words: words.length ? this.sameRun(words) : [], next: verse.lines[li + 1], upcoming: true };
      }
      const line = verse.lines[li];
      // the run of played measures, around this one, that belong to the line
      let qa = k, qb = k;
      while (qa > 0 && perf[qa - 1].mi >= line.from && perf[qa - 1].mi <= line.to && perf[qa - 1].mi < perf[qa].mi) qa--;
      while (qb + 1 < perf.length && perf[qb + 1].mi >= line.from && perf[qb + 1].mi <= line.to && perf[qb + 1].mi > perf[qb].mi) qb++;
      const words = plan.words.filter((w) => w.k >= qa && w.k <= qb);
      return { line: li, words, next: verse.lines[li + 1], upcoming: false };
    }

    sameRun(words) {
      const first = words[0].k;
      return words.filter((w) => w.k - first < 64 && w.k >= first);
    }

    draw(t) {
      const { job } = this;
      const { plan } = job;
      const a = t - LEAD;                       // seconds into the music
      const playing = a >= 0 && a < plan.total;
      const k = this.perfAt(Math.max(0, a));
      const kk = playing ? k : -1;
      const off = Math.round(this.scrollAt(Math.max(0, Math.min(a, plan.total)), k));
      if (k !== this.lyrK) { this.lyrK = k; this.lyr = this.lyricsAt(Math.max(0, a), k); }
      const lyr = this.lyr;
      let wi = -1;
      if (lyr && !lyr.upcoming && playing) for (let i = 0; i < lyr.words.length; i++) if (lyr.words[i].s <= a) wi = i;
      const prog = Math.round((W * Math.max(0, Math.min(a, plan.total))) / Math.max(0.001, plan.total));
      const key = `${off}|${kk}|${lyr ? lyr.line : -1}|${lyr && lyr.upcoming ? 'u' : ''}|${wi}|${prog}`;
      if (key === this.lastKey) return;     // nothing changed: keep the last picture
      this.lastKey = key;

      const g = this.g;
      g.fillStyle = '#ffffff';
      g.fillRect(0, 0, W, H);

      // the music
      g.save();
      g.beginPath();
      g.rect(0, MUSIC_TOP, W, MUSIC_H);
      g.clip();
      const { systems, measures, pages } = this.layout;
      systems.forEach((s, i) => {
        const dy = MUSIC_TOP + 8 + this.sysY[i] - off;
        if (dy + s.h < MUSIC_TOP || dy > MUSIC_TOP + MUSIC_H) return;
        const page = pages[s.page];
        g.drawImage(page, 0, s.y, page.width, s.h, this.x0, dy, page.width, s.h);
      });
      if (kk >= 0) {
        const m = measures[plan.perf[kk].mi - job.from];
        if (m) {
          const s = systems[m.sys];
          const dy = MUSIC_TOP + 8 + this.sysY[m.sys] - off;
          g.globalCompositeOperation = 'multiply';
          g.fillStyle = '#ffe36e';
          g.fillRect(this.x0 + m.x - 3, dy + PAD - 6, m.w + 6, s.h - 2 * PAD + 12);
          g.globalCompositeOperation = 'source-over';
        }
      }
      // fade out whatever is cut off at the top edge
      if (!this.fade) {
        this.fade = g.createLinearGradient(0, MUSIC_TOP, 0, MUSIC_TOP + 26);
        this.fade.addColorStop(0, 'rgba(255,255,255,1)');
        this.fade.addColorStop(1, 'rgba(255,255,255,0)');
      }
      g.fillStyle = this.fade;
      g.fillRect(0, MUSIC_TOP, W, 26);
      g.restore();

      // the title bar
      g.fillStyle = '#0b3a7e';
      g.fillRect(0, 0, W, HEADER);
      g.fillStyle = '#ffffff';
      g.textBaseline = 'alphabetic';
      g.textAlign = 'left';
      const right = playing ? `Measure ${job.measures[plan.perf[k].mi].n}` : '';
      g.font = `700 ${this.titleSize}px "Segoe UI", system-ui, Arial, sans-serif`;
      g.fillText(job.title, 24, 30);
      g.font = `400 ${this.partsSize}px "Segoe UI", system-ui, Arial, sans-serif`;
      g.fillStyle = '#d6e4ff';
      g.fillText(job.partsText, 24, 54);
      if (right) {
        g.fillStyle = '#ffffff';
        g.font = '600 22px "Segoe UI", system-ui, Arial, sans-serif';
        g.textAlign = 'right';
        g.fillText(right, W - 24, 40);
        g.textAlign = 'left';
      }

      // the lyrics
      const ly = H - LYRICS;
      g.fillStyle = '#16181d';
      g.fillRect(0, ly, W, LYRICS);
      if (lyr && lyr.words.length) {
        this.drawWords(lyr.words, wi, ly + 52, lyr.upcoming);
      }
      if (lyr && lyr.next) {
        g.fillStyle = '#9aa3b2';
        fitText(g, lyr.next.text, W - 80, 22, 14, 400);
        g.textAlign = 'center';
        g.fillText(lyr.next.text, W / 2, ly + 98);
        g.textAlign = 'left';
      }

      // how far through the piece
      g.fillStyle = '#2b2f38';
      g.fillRect(0, H - PROGRESS, W, PROGRESS);
      g.fillStyle = '#4f8cff';
      g.fillRect(0, H - PROGRESS, prog, PROGRESS);
    }

    drawWords(words, current, baseline, upcoming) {
      const g = this.g;
      const text = words.map((w) => w.text).join(' ');
      const size = fitText(g, text, W - 80, 36, 20, 600);
      const space = g.measureText(' ').width;
      let x = (W - g.measureText(text).width) / 2;
      words.forEach((w, i) => {
        if (upcoming) g.fillStyle = '#9aa3b2';
        else if (i === current) g.fillStyle = '#ffd84d';
        else if (i < current) g.fillStyle = '#ffffff';
        else g.fillStyle = '#c2c8d2';
        g.fillText(w.text, x, baseline);
        if (i === current && !upcoming) {
          g.fillRect(x, baseline + 6, g.measureText(w.text).width, 3);
        }
        x += g.measureText(w.text).width + space;
      });
      return size;
    }
  }

  // ---------- encoding ----------

  /**
   * job: { title, partsText, xml, staves, plan, measures, from, verse,
   *        renderAudio(rate, lead, tail) -> AudioBuffer }
   * onProgress(fraction, message), isCancelled()
   */
  async function make(job, onProgress, isCancelled) {
    const support = await codecs();
    if (!support) {
      const e = new Error('This browser cannot make videos. Use a recent Microsoft Edge or Google Chrome.');
      e.unsupported = true;
      throw e;
    }
    const check = () => { if (isCancelled()) throw new Cancelled('Cancelled.'); };

    onProgress(0, 'Engraving the music');
    const layout = await engrave(job.xml, job.staves);
    if (layout.measures.length < job.measureCount) {
      throw new Error('The engraved music has fewer measures than the score, so the video could not follow it.');
    }
    check();

    onProgress(0.03, 'Making the sound');
    const audio = await job.renderAudio(RATE, LEAD, TAIL);
    check();

    let failure = null;
    const muxer = new Mp4Muxer.Muxer({
      target: new Mp4Muxer.ArrayBufferTarget(),
      video: { codec: 'avc', width: W, height: H, frameRate: FPS },
      audio: { codec: support.audioCodec, numberOfChannels: 1, sampleRate: RATE },
      fastStart: 'in-memory',
      firstTimestampBehavior: 'offset',
    });

    const aenc = new AudioEncoder({ output: (c, m) => muxer.addAudioChunk(c, m), error: (e) => { failure = e; } });
    aenc.configure(support.audio);
    const pcm = audio.getChannelData(0);
    const CHUNK = 4800;
    for (let i = 0; i < pcm.length; i += CHUNK) {
      const n = Math.min(CHUNK, pcm.length - i);
      const ad = new AudioData({ format: 'f32-planar', sampleRate: RATE, numberOfFrames: n, numberOfChannels: 1,
        timestamp: Math.round((i * 1e6) / RATE), data: pcm.subarray(i, i + n) });
      aenc.encode(ad);
      ad.close();
    }
    await aenc.flush();
    aenc.close();
    if (failure) throw failure;
    check();

    const frames = new Frames(job, layout);
    const venc = new VideoEncoder({ output: (c, m) => muxer.addVideoChunk(c, m), error: (e) => { failure = e; } });
    venc.configure(support.video);
    const count = Math.ceil(audio.duration * FPS);
    const dur = Math.round(1e6 / FPS);
    for (let i = 0; i < count; i++) {
      if (failure) throw failure;
      check();
      frames.draw(i / FPS);
      const vf = new VideoFrame(frames.canvas, { timestamp: Math.round((i * 1e6) / FPS), duration: dur });
      venc.encode(vf, { keyFrame: i % (FPS * 2) === 0 });
      vf.close();
      while (venc.encodeQueueSize > 8) await new Promise((r) => setTimeout(r, 2));
      if (i % 30 === 0) {
        onProgress(0.05 + 0.93 * (i / count), 'Making the video');
        await new Promise((r) => setTimeout(r, 0));
      }
    }
    await venc.flush();
    venc.close();
    if (failure) throw failure;
    muxer.finalize();
    onProgress(1, 'Finished');
    return { blob: new Blob([muxer.target.buffer], { type: 'video/mp4' }), audioCodec: support.audioCodec };
  }

  return { make, codecs, Cancelled, LEAD, TAIL };
})();
