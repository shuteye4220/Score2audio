package io.github.shuteye4220.score2audio

import kotlin.math.PI
import kotlin.math.abs
import kotlin.math.exp
import kotlin.math.max
import kotlin.math.min
import kotlin.math.pow
import kotlin.math.sin
import kotlin.math.sqrt

/** The instrument sounds, the same as score2audio on a computer. */
object Sounds {
    val ids = listOf("piano", "organ", "flute", "reed")
    val names = listOf("Piano", "Organ (holds each note steadily)", "Soft flute", "Clarinet")

    const val TABLE = 2048
    private fun table(amps: DoubleArray): FloatArray {
        val t = FloatArray(TABLE)
        var peak = 0.0
        val raw = DoubleArray(TABLE) { i ->
            var s = 0.0
            for (h in 1 until amps.size) if (amps[h] != 0.0) s += amps[h] * sin(2 * PI * h * i / TABLE)
            s
        }
        for (v in raw) peak = max(peak, abs(v))
        for (i in 0 until TABLE) t[i] = (raw[i] / peak).toFloat()
        return t
    }

    val sine = table(doubleArrayOf(0.0, 1.0))
    val pianoBright = table(doubleArrayOf(0.0, 1.0, 0.55, 0.32, 0.22, 0.14, 0.09, 0.06, 0.04))
    val organ = table(doubleArrayOf(0.0, 1.0, 0.7, 0.35, 0.3, 0.12, 0.1, 0.0, 0.06))
    val flute = table(doubleArrayOf(0.0, 1.0, 0.12, 0.04))
    val reed = table(doubleArrayOf(0.0, 1.0, 0.0, 0.45, 0.0, 0.25, 0.0, 0.12, 0.0, 0.06))

    fun freq(midi: Int) = 440.0 * 2.0.pow((midi - 69) / 12.0)
}

/** One sounding note. */
private class Voice(val line: Line?, sound: String, midi: Int, startFrame: Long, durSec: Double, vel: Float, val rate: Int) {
    private val inc = Sounds.freq(midi) / rate * Sounds.TABLE
    private var phase = 0.0
    private var frame = -startFrame // frames since the note started (negative before)
    private val piano = sound == "piano"
    private val table = when (sound) { "organ" -> Sounds.organ; "flute" -> Sounds.flute; "reed" -> Sounds.reed; else -> Sounds.pianoBright }
    private val endFrame = (max(0.05, durSec) * rate).toLong()
    var finished = false

    // piano envelope: a bright part that fades fast and a round part that fades slowly
    private var e1 = 0.0
    private var e2 = 0.0
    private val a1 = (0.004 * rate).toLong()
    private val a2 = (0.006 * rate).toLong()
    private val f = Sounds.freq(midi)
    private val decay = min(6.0, max(0.7, 3.2 * sqrt(220.0 / f)))
    private val d1 = exp(-1.0 / (decay * 0.18 * rate))
    private val d2 = exp(-1.0 / (decay * 0.55 * rate))
    private val peak1 = 0.35 * vel
    private val peak2 = 0.45 * vel
    private var release = 1.0
    private val relPiano = exp(-1.0 / (0.06 * rate))

    // sustained sounds: attack, hold, then fade, ending a little early so
    // repeated notes are heard separately
    private val attack = when (sound) { "flute" -> 0.04; "reed" -> 0.025; else -> 0.015 }
    private val level = (when (sound) { "flute" -> 0.45; "reed" -> 0.3; else -> 0.28 }) * vel
    private val attackFrames = (attack * rate).toLong()
    private val offFrame = max(attackFrames + (0.01 * rate).toLong(), endFrame - (min(0.05, durSec * 0.12) * rate).toLong())
    private val relSustain = exp(-1.0 / (0.025 * rate))
    private var env = 0.0

    /** Adds this voice into out[from until from+n] with gain g. */
    fun render(out: FloatArray, from: Int, n: Int, g: Float) {
        for (i in from until from + n) {
            if (frame < 0) { frame++; continue }
            val idx = phase.toInt()
            val frac = (phase - idx).toFloat()
            val nxt = if (idx + 1 == Sounds.TABLE) 0 else idx + 1
            var v: Double
            if (piano) {
                e1 = if (frame < a1) peak1 * frame / a1 else if (frame == a1) peak1 else e1 * d1
                e2 = if (frame < a2) peak2 * frame / a2 else if (frame == a2) peak2 else e2 * d2
                val bright = table[idx] + (table[nxt] - table[idx]) * frac
                val round = Sounds.sine[idx] + (Sounds.sine[nxt] - Sounds.sine[idx]) * frac
                v = e1 * bright + e2 * round
                if (frame >= endFrame) release *= relPiano
                v *= release
                if (frame > endFrame + rate / 2) { finished = true; return }
            } else {
                env = when {
                    frame < attackFrames -> level * frame / attackFrames
                    frame < offFrame -> level
                    else -> env * relSustain
                }
                v = env * (table[idx] + (table[nxt] - table[idx]) * frac)
                if (frame > offFrame + (0.3 * rate).toLong()) { finished = true; return }
            }
            out[i] += (v * g).toFloat()
            phase += inc
            if (phase >= Sounds.TABLE) phase -= Sounds.TABLE
            frame++
        }
    }
}

private class ClickVoice(startFrame: Long, accent: Boolean, val rate: Int) {
    private var frame = -startFrame
    private val period = rate / (if (accent) 1760.0 else 1175.0)
    private val peak = if (accent) 0.22 else 0.14
    private val decay = exp(-1.0 / (0.012 * rate))
    private val attack = (0.002 * rate).toLong()
    private var env = 0.0
    var finished = false

    fun render(out: FloatArray, from: Int, n: Int, g: Float) {
        for (i in from until from + n) {
            if (frame < 0) { frame++; continue }
            env = if (frame < attack) peak * frame / attack else if (frame == attack) peak else env * decay
            val square = if ((frame % period.toLong().coerceAtLeast(1)) < period / 2) 1.0 else -1.0
            out[i] += (env * square * g).toFloat()
            frame++
            if (frame > rate / 10) { finished = true; return }
        }
    }
}

/**
 * Turns a plan into sound, block by block. Used both for live playing and
 * for saving audio files. [solo] plays one line only, whatever is chosen.
 */
class Renderer(private val plan: Plan, startSec: Double, private val rate: Int, private val solo: Line? = null) {
    private var pos: Long = (startSec * rate).toLong() // frames from the plan's start
    private var ei = plan.events.indexOfFirst { it.s >= startSec - 0.001 }.let { if (it < 0) plan.events.size else it }
    private var ci = plan.clicks.indexOfFirst { it.s >= startSec - 0.001 }.let { if (it < 0) plan.clicks.size else it }
    private val voices = ArrayList<Voice>()
    private val clicks = ArrayList<ClickVoice>()
    private val endFrame = ((plan.total + 0.6) * rate).toLong()

    val done: Boolean get() = pos > endFrame && voices.isEmpty() && clicks.isEmpty()

    private fun isOn(line: Line?) = when {
        line == null -> true
        solo != null -> solo === line
        else -> line.enabled
    }

    private fun gain(line: Line?): Float {
        if (line == null) return 1f
        if (!isOn(line)) return 0f
        val v = line.volume / 100f
        return v * v
    }

    /** Fills out[0 until n] with the next n frames. */
    fun render(out: FloatArray, n: Int) {
        java.util.Arrays.fill(out, 0, n, 0f)
        val blockEnd = pos + n
        while (ei < plan.events.size && (plan.events[ei].s * rate).toLong() < blockEnd) {
            val e = plan.events[ei++]
            if (!isOn(e.line)) continue
            val start = max(0L, (e.s * rate).toLong() - pos)
            val shift = e.line?.shift ?: 0
            voices += Voice(e.line, e.sound ?: e.line?.sound ?: "piano", e.midi + shift, start, e.d, e.vel, rate)
        }
        while (ci < plan.clicks.size && (plan.clicks[ci].s * rate).toLong() < blockEnd) {
            val c = plan.clicks[ci++]
            clicks += ClickVoice(max(0L, (c.s * rate).toLong() - pos), c.accent, rate)
        }
        val it = voices.iterator()
        while (it.hasNext()) {
            val v = it.next()
            v.render(out, 0, n, gain(v.line))
            if (v.finished) it.remove()
        }
        val ct = clicks.iterator()
        while (ct.hasNext()) {
            val c = ct.next()
            c.render(out, 0, n, 0.7f)
            if (c.finished) ct.remove()
        }
        // gentle limiter: keeps chords from clipping
        for (i in 0 until n) {
            val x = out[i] * 0.55f
            out[i] = if (x > 1f) 1f else if (x < -1f) -1f else x * (1.5f - 0.5f * x * x)
        }
        pos = blockEnd
    }

    /** Converts float samples to 16-bit. */
    companion object {
        fun toPcm16(src: FloatArray, n: Int, dst: ShortArray) {
            for (i in 0 until n) dst[i] = (src[i].coerceIn(-1f, 1f) * 32767f).toInt().toShort()
        }
    }
}
