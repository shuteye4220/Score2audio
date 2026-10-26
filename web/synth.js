'use strict';
// Simple instrument sounds made with the Web Audio API. Works with both a
// live AudioContext and an OfflineAudioContext (for saving audio files).

const Synth = (() => {
  const SOUNDS = [
    { id: 'piano', name: 'Piano' },
    { id: 'organ', name: 'Organ (holds each note steadily)' },
    { id: 'flute', name: 'Soft flute' },
    { id: 'reed', name: 'Clarinet' },
  ];
  const waveCache = new WeakMap();

  function wave(ctx, kind) {
    let c = waveCache.get(ctx);
    if (!c) { c = {}; waveCache.set(ctx, c); }
    if (c[kind]) return c[kind];
    const tables = {
      pianoBright: [0, 1, 0.55, 0.32, 0.22, 0.14, 0.09, 0.06, 0.04],
      organ: [0, 1, 0.7, 0.35, 0.3, 0.12, 0.1, 0, 0.06],
      flute: [0, 1, 0.12, 0.04],
      reed: [0, 1, 0, 0.45, 0, 0.25, 0, 0.12, 0, 0.06],
    };
    const amps = tables[kind];
    const real = new Float32Array(amps.length);
    const imag = new Float32Array(amps);
    c[kind] = ctx.createPeriodicWave(real, imag, { disableNormalization: false });
    return c[kind];
  }

  function freq(midi) { return 440 * Math.pow(2, (midi - 69) / 12); }

  // Schedules one note. vel is 0..1. dest is an AudioNode.
  function note(ctx, dest, sound, midi, when, dur, vel) {
    const f = freq(midi);
    const end = when + Math.max(0.05, dur);
    const out = ctx.createGain();
    out.gain.value = 1;
    out.connect(dest);
    const oscs = [];
    if (sound === 'piano') {
      const decay = Math.min(6, Math.max(0.7, 3.2 * Math.sqrt(220 / f)));
      const o1 = ctx.createOscillator(); o1.setPeriodicWave(wave(ctx, 'pianoBright')); o1.frequency.value = f;
      const o2 = ctx.createOscillator(); o2.type = 'sine'; o2.frequency.value = f;
      const g1 = ctx.createGain(), g2 = ctx.createGain();
      g1.gain.setValueAtTime(0, when);
      g1.gain.linearRampToValueAtTime(0.35 * vel, when + 0.004);
      g1.gain.setTargetAtTime(0, when + 0.004, decay * 0.18);
      g2.gain.setValueAtTime(0, when);
      g2.gain.linearRampToValueAtTime(0.45 * vel, when + 0.006);
      g2.gain.setTargetAtTime(0, when + 0.006, decay * 0.55);
      o1.connect(g1).connect(out); o2.connect(g2).connect(out);
      oscs.push(o1, o2);
      out.gain.setValueAtTime(1, end);
      out.gain.setTargetAtTime(0, end, 0.06);
    } else {
      const o = ctx.createOscillator();
      o.setPeriodicWave(wave(ctx, sound in { organ: 1, flute: 1, reed: 1 } ? sound : 'organ'));
      o.frequency.value = f;
      const g = ctx.createGain();
      const attack = sound === 'flute' ? 0.04 : sound === 'reed' ? 0.025 : 0.015;
      const level = (sound === 'flute' ? 0.45 : sound === 'reed' ? 0.3 : 0.28) * vel;
      // end a little early so repeated notes are heard as separate notes
      const off = Math.max(when + attack + 0.01, end - Math.min(0.05, dur * 0.12));
      g.gain.setValueAtTime(0, when);
      g.gain.linearRampToValueAtTime(level, when + attack);
      g.gain.setValueAtTime(level, off);
      g.gain.setTargetAtTime(0, off, 0.025);
      o.connect(g).connect(out);
      oscs.push(o);
    }
    const stopAt = end + 0.5;
    for (const o of oscs) { o.start(when); o.stop(stopAt); }
    oscs[0].onended = () => { try { out.disconnect(); } catch (e) { /* already gone */ } };
    return stopAt;
  }

  function click(ctx, dest, when, accent) {
    const o = ctx.createOscillator();
    o.type = 'square';
    o.frequency.value = accent ? 1760 : 1175;
    const g = ctx.createGain();
    g.gain.setValueAtTime(0, when);
    g.gain.linearRampToValueAtTime(accent ? 0.22 : 0.14, when + 0.002);
    g.gain.setTargetAtTime(0, when + 0.002, 0.012);
    o.connect(g).connect(dest);
    o.start(when); o.stop(when + 0.1);
  }

  // Encodes mono or stereo float data as a 16-bit WAV file.
  function toWav(buffer) {
    const ch = buffer.numberOfChannels, rate = buffer.sampleRate, len = buffer.length;
    const data = new DataView(new ArrayBuffer(44 + len * ch * 2));
    const str = (o, s) => { for (let i = 0; i < s.length; i++) data.setUint8(o + i, s.charCodeAt(i)); };
    str(0, 'RIFF'); data.setUint32(4, 36 + len * ch * 2, true); str(8, 'WAVE');
    str(12, 'fmt '); data.setUint32(16, 16, true); data.setUint16(20, 1, true); data.setUint16(22, ch, true);
    data.setUint32(24, rate, true); data.setUint32(28, rate * ch * 2, true); data.setUint16(32, ch * 2, true); data.setUint16(34, 16, true);
    str(36, 'data'); data.setUint32(40, len * ch * 2, true);
    const chans = []; for (let c = 0; c < ch; c++) chans.push(buffer.getChannelData(c));
    let o = 44;
    for (let i = 0; i < len; i++) {
      for (let c = 0; c < ch; c++) {
        const s = Math.max(-1, Math.min(1, chans[c][i]));
        data.setInt16(o, s < 0 ? s * 0x8000 : s * 0x7fff, true); o += 2;
      }
    }
    return new Blob([data], { type: 'audio/wav' });
  }

  return { SOUNDS, note, click, toWav, freq };
})();
