package io.github.shuteye4220.score2audio

import org.json.JSONArray
import org.json.JSONObject
import kotlin.math.abs
import kotlin.math.round

// A score read from a score2audio score file (.s2a). The format is
// described in docs/s2a-format.md in the score2audio project.

class Measure(val n: String, val t: Double, val d: Double, val beats: Int, val beatType: Int, val newSystem: Boolean)

class TempoPoint(val t: Double, val bpm: Double)

class NoteEvent(
    val t: Double, val d: Double, val m: Int, val mi: Int,
    val name: String, val dur: String, val lyric: String,
)

class Line(
    val id: String,
    val name: String,
    val hint: String,
    val kind: String,
    val part: String,
    val primary: Boolean,
    val isDefault: Boolean,
    val clef: String,
    val low: String,
    val high: String,
    val transpose: Int,
    val transposeNote: String,
    val events: List<NoteEvent>,
) {
    val byMeasure: Map<Int, List<NoteEvent>> = events.groupBy { it.mi }

    // the user's choices
    @Volatile var enabled = isDefault
    @Volatile var volume = defaultVolume(kind)
    @Volatile var sound = "piano"
    @Volatile var shift = transpose
    var label = ""

    val displayName: String get() = label.ifBlank { name }

    /** Shown under the line's name: what it usually is, its clef and range. */
    fun details(): String {
        val parts = mutableListOf<String>()
        if (hint.isNotBlank()) parts += hint
        if (clef.isNotBlank() && !name.contains(clef)) parts += clef
        parts += if (low.isNotBlank()) "notes from $low to $high as printed" else "no notes"
        var s = parts.joinToString(", ")
        if (transposeNote.isNotBlank()) s += ". $transposeNote"
        return s
    }

    companion object {
        fun defaultVolume(kind: String) = when (kind) {
            "voice" -> 80
            "instrument", "other" -> 75
            "keyboard" -> 55
            else -> 70
        }
    }
}

class Word(val t: Double, val mi: Int, val text: String)
class LyricLine(val from: Int, val to: Int, val text: String)
class Verse(val name: String, val words: List<Word>, val lines: List<LyricLine>)

class Options {
    var tempo = 90
    var from = "1"
    var to = "1"
    var loop = false
    var countIn = true
    var metronome = false
    var repeats = true
    var sayMeasures = false
    var sayLyrics = false
    var verse = 0
    var notesLine = ""
}

class Score(
    val id: String,
    val title: String,
    val sourceFile: String,
    val measures: List<Measure>,
    val order: List<Int>,
    val hasRepeats: Boolean,
    val tempo: List<TempoPoint>,
    val tempoFromScore: Boolean,
    val lines: List<Line>,
    val verses: List<Verse>,
    val warnings: List<String>,
) {
    val opts = Options()

    val lastNumber: String get() = measures.last().n

    /** Length of one beat in quarter notes, for a measure's time signature. */
    fun beatQn(m: Measure): Double =
        if (m.beatType == 8 && m.beats % 3 == 0 && m.beats > 3) 1.5 else 4.0 / m.beatType

    val beatQn0: Double get() = beatQn(measures[0])

    /** The tempo the score asks for, in beats per minute. */
    val scoreTempo: Int get() = round(tempo[0].bpm / beatQn0).toInt().coerceIn(20, 300)

    /** User tempo divided by score tempo, applied to every tempo in the score. */
    val factor: Double get() = opts.tempo * beatQn0 / tempo[0].bpm

    fun measureIndex(num: String): Int {
        val s = num.trim()
        val i = measures.indexOfFirst { it.n == s }
        if (i >= 0) return i
        val n = s.toIntOrNull() ?: return -1
        return (n - 1).coerceIn(0, measures.size - 1)
    }

    fun range(): IntRange {
        var f = measureIndex(opts.from)
        var t = measureIndex(opts.to)
        if (f < 0) f = 0
        if (t < 0) t = measures.size - 1
        if (t < f) { val x = f; f = t; t = x }
        return f..t
    }

    /** Seconds between two points of the score (in quarter notes). */
    fun secBetween(a: Double, b: Double): Double {
        if (b <= a) return 0.0
        var s = 0.0
        val f = factor
        for (i in tempo.indices) {
            val lo = maxOf(a, tempo[i].t)
            val hi = minOf(b, if (i + 1 < tempo.size) tempo[i + 1].t else Double.MAX_VALUE)
            if (hi > lo) s += (hi - lo) * 60.0 / (tempo[i].bpm * f)
        }
        return s
    }

    fun beatName(q: Double) = when (q) {
        4.0 -> "whole note"; 3.0 -> "dotted half note"; 2.0 -> "half note"
        1.5 -> "dotted quarter note"; 0.5 -> "eighth note"; else -> "quarter note"
    }

    fun summary(): String {
        val groups = listOf("voice" to "voice", "instrument" to "instrument", "other" to "other line", "keyboard" to "keyboard line")
        val parts = groups.mapNotNull { (k, word) ->
            val n = lines.count { it.kind == k && it.primary }
            if (n == 0) null else "$n $word${if (n == 1) "" else "s"}"
        }
        val words = verses.sumOf { it.words.size }
        return "${measures.size} measures, ${parts.joinToString(", ").ifEmpty { "no lines" }}, " +
            (if (words > 0) "lyrics found." else "no lyrics found.")
    }

    // ---- preferences, in the same form score2audio uses on the computer ----

    fun prefsJson(): JSONObject {
        val o = JSONObject()
            .put("tempo", opts.tempo).put("from", opts.from).put("to", opts.to)
            .put("loop", opts.loop).put("countIn", opts.countIn).put("metronome", opts.metronome)
            .put("repeats", opts.repeats).put("sayMeasures", opts.sayMeasures).put("sayLyrics", opts.sayLyrics)
            .put("verse", opts.verse).put("notesLine", opts.notesLine)
        val ls = JSONObject()
        for (l in lines) {
            ls.put(l.id, JSONObject().put("enabled", l.enabled).put("volume", l.volume).put("sound", l.sound)
                .put("shift", l.shift).put("label", l.label))
        }
        return JSONObject().put("opts", o).put("lines", ls)
    }

    fun applyPrefs(p: JSONObject?) {
        opts.tempo = scoreTempo
        opts.from = measures.first().n
        opts.to = lastNumber
        if (p == null) return
        p.optJSONObject("opts")?.let { o ->
            opts.tempo = o.optInt("tempo", opts.tempo).coerceIn(20, 300)
            opts.from = o.optString("from", opts.from)
            opts.to = o.optString("to", opts.to)
            opts.loop = o.optBoolean("loop", false)
            opts.countIn = o.optBoolean("countIn", true)
            opts.metronome = o.optBoolean("metronome", false)
            opts.repeats = o.optBoolean("repeats", true)
            opts.sayMeasures = o.optBoolean("sayMeasures", false)
            opts.sayLyrics = o.optBoolean("sayLyrics", false)
            opts.verse = o.optInt("verse", 0).let { if (it in verses.indices) it else 0 }
            opts.notesLine = o.optString("notesLine", "")
        }
        val lp = p.optJSONObject("lines") ?: return
        for (l in lines) {
            val o = lp.optJSONObject(l.id) ?: continue
            l.enabled = o.optBoolean("enabled", l.enabled)
            l.volume = o.optInt("volume", l.volume).coerceIn(0, 100)
            l.sound = o.optString("sound", l.sound).let { if (it in Sounds.ids) it else "piano" }
            l.shift = if (o.has("shift")) o.optInt("shift", l.shift) else if (o.has("octave")) o.optInt("octave") * 12 else l.shift
            l.label = o.optString("label", "")
        }
    }

    // ---- text versions of the music ----

    fun measureText(line: Line, mi: Int): String {
        val meas = measures[mi]
        val end = meas.t + meas.d
        var cursor = meas.t
        val parts = mutableListOf<String>()
        for (back in (mi - 1) downTo maxOf(0, mi - 8)) {
            for (e in line.byMeasure[back].orEmpty()) {
                if (e.t + e.d > meas.t + 0.01) cursor = maxOf(cursor, minOf(end, e.t + e.d))
            }
        }
        if (cursor > meas.t + 0.01) parts += "held note continues"
        val groups = line.byMeasure[mi].orEmpty().groupBy { round(it.t * 1000) }.toSortedMap().values
        for (g in groups) {
            val t = g.first().t
            if (t > cursor + 0.02) parts += "${qnName(t - cursor)} rest"
            val names = g.sortedBy { it.m }.map { noteName(it, line.shift) }
            val longest = g.maxBy { it.d }
            var txt = "${names.joinToString(" with ")}, ${longest.dur}"
            if (longest.t + longest.d > end + 0.02) txt += ", held into the next measure"
            parts += txt
            cursor = maxOf(cursor, longest.t + longest.d)
        }
        if (groups.isEmpty() && cursor <= meas.t + 0.01) return "rest for the whole measure"
        if (cursor < end - 0.02) parts += "${qnName(end - cursor)} rest"
        return parts.joinToString("; ")
    }

    fun wordsIn(mi: Int, line: Line?): String {
        val v = verses.getOrNull(opts.verse)
        val w = v?.words?.filter { it.mi == mi }?.joinToString(" ") { it.text }.orEmpty()
        if (w.isNotEmpty() || line == null) return w
        return line.byMeasure[mi].orEmpty().mapNotNull { it.lyric.ifBlank { null } }.joinToString(" ")
    }

    companion object {
        private val PC_NAMES = listOf("C", "C sharp", "D", "E flat", "E", "F", "F sharp", "G", "A flat", "A", "B flat", "B")

        fun noteName(e: NoteEvent, shift: Int): String {
            if (shift == 0) return e.name
            val m = e.m + shift
            return "${PC_NAMES[((m % 12) + 12) % 12]} ${Math.floorDiv(m, 12) - 1}"
        }

        fun midiName(m: Int) = "${PC_NAMES[((m % 12) + 12) % 12]} ${Math.floorDiv(m, 12) - 1}"

        private val QN = listOf(8.0 to "two whole notes", 6.0 to "dotted whole", 4.0 to "whole", 3.0 to "dotted half",
            2.0 to "half", 1.5 to "dotted quarter", 1.0 to "quarter", 0.75 to "dotted eighth", 0.5 to "eighth",
            0.25 to "sixteenth", 2.0 / 3 to "triplet quarter", 1.0 / 3 to "triplet eighth")

        fun qnName(q: Double): String {
            for ((v, n) in QN) if (abs(q - v) < 0.02) return n
            var rest = q
            val parts = mutableListOf<String>()
            for ((v, n) in QN) {
                while (rest >= v - 0.02 && parts.size < 4) { parts += n; rest -= v }
            }
            return if (parts.isNotEmpty()) parts.joinToString(" plus ") else "${round(q * 100) / 100} beats"
        }

        /** Reads score.json from a .s2a file. */
        fun fromJson(id: String, root: JSONObject): Score {
            if (root.optString("format") != "score2audio-score") throw ScoreFileException("That is not a score2audio score file.")
            if (root.optInt("formatVersion", 0) > 1) {
                throw ScoreFileException("That score file was made by a newer version of score2audio. Please update the app.")
            }
            val m = root.optJSONObject("model") ?: throw ScoreFileException("That score file does not contain any music.")
            val measures = m.getJSONArray("measures").map { o ->
                Measure(o.getString("n"), o.getDouble("t"), o.getDouble("d"), o.optInt("beats", 4), o.optInt("beatType", 4), o.optBoolean("sys"))
            }
            if (measures.isEmpty()) throw ScoreFileException("That score file does not contain any music.")
            val order = m.optJSONArray("order")?.let { a -> (0 until a.length()).map { a.getInt(it) } } ?: measures.indices.toList()
            var tempo = m.optJSONArray("tempo")?.map { TempoPoint(it.getDouble("t"), it.getDouble("bpm")) }.orEmpty()
            if (tempo.isEmpty() || tempo[0].t > 0) tempo = listOf(TempoPoint(0.0, tempo.firstOrNull()?.bpm ?: 90.0)) + tempo
            val lines = m.optJSONArray("lines")?.map { o ->
                Line(
                    id = o.getString("id"), name = o.optString("name"), hint = o.optString("hint"),
                    kind = o.optString("kind", "other"), part = o.optString("part", "single"),
                    primary = o.optBoolean("primary", true), isDefault = o.optBoolean("default", false),
                    clef = o.optString("clef"), low = o.optString("low"), high = o.optString("high"),
                    transpose = o.optInt("transpose", 0), transposeNote = o.optString("transposeNote"),
                    events = o.optJSONArray("events")?.map { e ->
                        NoteEvent(e.getDouble("t"), e.getDouble("d"), e.getInt("m"), e.getInt("mi"),
                            e.optString("name"), e.optString("dur"), e.optString("ly"))
                    }.orEmpty(),
                )
            }.orEmpty()
            val verses = m.optJSONArray("verses")?.map { v ->
                Verse(
                    v.optString("name", "Lyrics"),
                    v.optJSONArray("words")?.map { Word(it.getDouble("t"), it.getInt("mi"), it.getString("text")) }.orEmpty(),
                    v.optJSONArray("lines")?.map { LyricLine(it.getInt("from"), it.getInt("to"), it.getString("text")) }.orEmpty(),
                )
            }.orEmpty()
            val warnings = m.optJSONArray("warnings")?.let { a -> (0 until a.length()).map { a.getString(it) } }.orEmpty()
            val title = root.optString("title").ifBlank { m.optString("title", "Score") }
            val score = Score(id, title, root.optString("sourceFile"), measures, order, m.optBoolean("hasRepeats"),
                tempo, m.optBoolean("tempoFromScore"), lines, verses, warnings)
            score.applyPrefs(root.optJSONObject("prefs"))
            return score
        }
    }
}

class ScoreFileException(message: String) : Exception(message)

private fun <T> JSONArray.map(f: (JSONObject) -> T): List<T> = (0 until length()).map { f(getJSONObject(it)) }
