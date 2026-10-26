package io.github.shuteye4220.score2audio

import android.content.ContentValues
import android.content.Context
import android.media.AudioAttributes
import android.media.AudioFormat
import android.media.AudioTrack
import android.os.Environment
import android.os.Handler
import android.os.Looper
import android.provider.MediaStore
import java.io.OutputStream
import java.nio.ByteBuffer
import java.nio.ByteOrder
import kotlin.math.max
import kotlin.math.min

/** Plays scores through the speaker or headphones. One at a time. */
object Player {
    const val RATE = 44100
    private const val BLOCK = 1024

    enum class Mode { NONE, SCORE, MEASURE, PITCHES }

    private class Resume(val k: Int, val frac: Double, val range: IntRange)

    private val main = Handler(Looper.getMainLooper())
    private var track: AudioTrack? = null
    private var thread: Thread? = null
    @Volatile private var running = false
    private var generation = 0
    private var startSec = 0.0
    private var resume: Resume? = null
    private var score: Score? = null

    var mode = Mode.NONE
        private set
    var plan: Plan? = null
        private set

    /** Called on the main thread when playing ends by itself. */
    var onEnded: ((Mode) -> Unit)? = null

    val isPlayingScore get() = mode == Mode.SCORE
    val canContinue get() = mode == Mode.NONE && resume != null

    /** Seconds into the current plan. */
    fun now(): Double {
        val t = track ?: return 0.0
        return startSec + t.playbackHeadPosition.toDouble() / RATE
    }

    private fun startPlan(p: Plan, from: Double, solo: Line?, m: Mode) {
        stopAudio()
        val gen = ++generation
        val minBuf = AudioTrack.getMinBufferSize(RATE, AudioFormat.CHANNEL_OUT_MONO, AudioFormat.ENCODING_PCM_16BIT)
        val t = AudioTrack.Builder()
            .setAudioAttributes(
                AudioAttributes.Builder().setUsage(AudioAttributes.USAGE_MEDIA)
                    .setContentType(AudioAttributes.CONTENT_TYPE_MUSIC).build()
            )
            .setAudioFormat(
                AudioFormat.Builder().setEncoding(AudioFormat.ENCODING_PCM_16BIT).setSampleRate(RATE)
                    .setChannelMask(AudioFormat.CHANNEL_OUT_MONO).build()
            )
            .setBufferSizeInBytes(max(minBuf, RATE / 5 * 2))
            .setTransferMode(AudioTrack.MODE_STREAM)
            .build()
        track = t
        plan = p
        startSec = from
        mode = m
        running = true
        t.play()
        val renderer = Renderer(p, from, RATE, solo)
        thread = Thread({
            val f = FloatArray(BLOCK)
            val s = ShortArray(BLOCK)
            var written = 0L
            try {
                while (running && !renderer.done) {
                    renderer.render(f, BLOCK)
                    Renderer.toPcm16(f, BLOCK, s)
                    var off = 0
                    while (off < BLOCK && running) {
                        val n = t.write(s, off, BLOCK - off)
                        if (n <= 0) break
                        off += n
                    }
                    written += BLOCK
                }
                while (running && t.playbackHeadPosition < written) Thread.sleep(20)
            } catch (e: Exception) {
                // the track was released while writing; nothing to do
            }
            if (running) main.post { if (gen == generation) finished() }
        }, "score2audio-player").apply { priority = Thread.MAX_PRIORITY; start() }
    }

    private fun stopAudio() {
        running = false
        generation++
        track?.let {
            try { it.pause(); it.flush() } catch (e: Exception) {}
        }
        thread?.join(500)
        thread = null
        track?.release()
        track = null
        mode = Mode.NONE
    }

    private fun finished() {
        val m = mode
        val sc = score
        stopAudio()
        if (m == Mode.SCORE && sc != null && sc.opts.loop) {
            startPlan(Plan.build(sc, sc.range(), false, sc.opts.metronome), -0.4, null, Mode.SCORE)
            return
        }
        if (m == Mode.SCORE) resume = null
        onEnded?.invoke(m)
    }

    /** Starts the chosen lines, or continues after a pause. Returns false when no line is chosen. */
    fun play(sc: Score): Boolean {
        if (sc.lines.none { it.enabled }) return false
        score = sc
        val range = sc.range()
        val r = resume
        resume = null
        if (r != null && r.range == range) {
            val p = Plan.build(sc, range, false, sc.opts.metronome)
            val pm = p.perf[min(r.k, p.perf.size - 1)]
            startPlan(p, pm.sec + r.frac * pm.dur, null, Mode.SCORE)
        } else {
            startPlan(Plan.build(sc, range, sc.opts.countIn, sc.opts.metronome), 0.0, null, Mode.SCORE)
        }
        return true
    }

    /** Pauses, remembering the place. Returns the printed measure number. */
    fun pause(): String? {
        val p = plan ?: return null
        val sc = score ?: return null
        if (mode != Mode.SCORE) { stop(); return null }
        val t = now()
        val k = p.measureAt(t)
        val pm = p.perf.getOrNull(k)
        val frac = if (pm != null && pm.dur > 0) ((t - pm.sec) / pm.dur).coerceIn(0.0, 1.0) else 0.0
        stopAudio()
        resume = Resume(max(0, k), frac, p.range)
        return pm?.let { sc.measures[it.mi].n }
    }

    fun stop() {
        stopAudio()
        resume = null
    }

    /** Called when the range changes: the next Play starts from the beginning of it. */
    fun forgetPlace() { resume = null }

    /** Starts again at the same place (after a tempo change). */
    fun restartHere(sc: Score) {
        val p = plan ?: return
        if (mode != Mode.SCORE) return
        val t = now()
        val k = p.measureAt(t)
        val pm = p.perf.getOrNull(k) ?: return
        val frac = if (pm.dur > 0) ((t - pm.sec) / pm.dur).coerceIn(0.0, 1.0) else 0.0
        val np = Plan.build(sc, p.range, false, sc.opts.metronome)
        val npm = np.perf[min(k, np.perf.size - 1)]
        startPlan(np, npm.sec + frac * npm.dur, null, Mode.SCORE)
    }

    /** Plays one measure of one line. Returns false if the line rests there. */
    fun playMeasure(sc: Score, line: Line, mi: Int): Boolean {
        resume = null
        score = sc
        val p = Plan.build(sc, mi..mi, false, false, only = line)
        if (p.events.isEmpty()) return false
        startPlan(p, 0.0, line, Mode.MEASURE)
        return true
    }

    /** Plays the first note of each chosen line. Returns a description, or null. */
    fun startingNotes(sc: Score): String? {
        resume = null
        score = sc
        val (p, desc) = Plan.startingNotes(sc, sc.lines.filter { it.enabled }) ?: return null
        startPlan(p, 0.0, null, Mode.PITCHES)
        return desc
    }

    // ---------- saving audio files ----------

    /**
     * Renders the chosen lines to a WAV file in Music/score2audio.
     * Calls done(fileDescription, errorMessage) on the main thread.
     */
    fun saveWav(ctx: Context, sc: Score, done: (String?, String?) -> Unit) {
        val chosen = sc.lines.filter { it.enabled }
        if (chosen.isEmpty()) { done(null, "No lines are chosen."); return }
        val plan = Plan.build(sc, sc.range(), false, sc.opts.metronome)
        val r = sc.range()
        val what = if (chosen.size == 1) chosen[0].displayName else "${chosen.size} lines"
        val name = "${sc.title} - $what - m${sc.measures[r.first].n}-${sc.measures[r.last].n} - ${sc.opts.tempo}bpm"
            .replace(Regex("[\\\\/:*?\"<>|()]+"), "").replace(Regex("\\s+"), " ").trim() + ".wav"
        val app = ctx.applicationContext
        Thread {
            try {
                val rate = 32000
                val values = ContentValues().apply {
                    put(MediaStore.Audio.Media.DISPLAY_NAME, name)
                    put(MediaStore.Audio.Media.MIME_TYPE, "audio/x-wav")
                    put(MediaStore.Audio.Media.RELATIVE_PATH, Environment.DIRECTORY_MUSIC + "/score2audio")
                    put(MediaStore.Audio.Media.IS_PENDING, 1)
                }
                val resolver = app.contentResolver
                val uri = resolver.insert(MediaStore.Audio.Media.getContentUri(MediaStore.VOLUME_EXTERNAL_PRIMARY), values)
                    ?: throw IllegalStateException("The Music folder could not be written to.")
                resolver.openOutputStream(uri)?.use { out -> writeWav(out, plan, rate) }
                    ?: throw IllegalStateException("The file could not be created.")
                values.clear()
                values.put(MediaStore.Audio.Media.IS_PENDING, 0)
                resolver.update(uri, values, null, null)
                main.post { done("Music/score2audio/$name", null) }
            } catch (e: Exception) {
                main.post { done(null, e.message ?: e.toString()) }
            }
        }.start()
    }

    private fun writeWav(out: OutputStream, plan: Plan, rate: Int) {
        val lead = 0.2
        val frames = ((plan.total + lead + 1.0) * rate).toLong()
        val dataBytes = frames * 2
        val h = ByteBuffer.allocate(44).order(ByteOrder.LITTLE_ENDIAN)
        h.put("RIFF".toByteArray()).putInt((36 + dataBytes).toInt()).put("WAVE".toByteArray())
        h.put("fmt ".toByteArray()).putInt(16).putShort(1).putShort(1).putInt(rate).putInt(rate * 2).putShort(2).putShort(16)
        h.put("data".toByteArray()).putInt(dataBytes.toInt())
        out.write(h.array())
        val renderer = Renderer(plan, -lead, rate)
        val f = FloatArray(BLOCK)
        val bytes = ByteBuffer.allocate(BLOCK * 2).order(ByteOrder.LITTLE_ENDIAN)
        var left = frames
        while (left > 0) {
            val n = min(BLOCK.toLong(), left).toInt()
            renderer.render(f, n)
            bytes.clear()
            for (i in 0 until n) bytes.putShort((f[i].coerceIn(-1f, 1f) * 32767f).toInt().toShort())
            out.write(bytes.array(), 0, n * 2)
            left -= n
        }
    }
}
