package io.github.shuteye4220.score2audio

import kotlin.math.max
import kotlin.math.roundToInt

/** One note to play: start and length in seconds, the line it belongs to. */
class PlanEvent(val s: Double, val d: Double, val midi: Int, val line: Line?, val vel: Float = 0.9f, val sound: String? = null)
class Click(val s: Double, val accent: Boolean)
class PerfMeasure(val mi: Int, val sec: Double, val dur: Double)
class TimedWord(val s: Double, val text: String)

/** Everything to be played, worked out ahead of time. */
class Plan(
    val perf: List<PerfMeasure>,
    val events: List<PlanEvent>,
    val clicks: List<Click>,
    val words: List<TimedWord>,
    val total: Double,
    val countInSec: Double,
    val range: IntRange,
) {
    /** Index into perf of the measure playing at time s. */
    fun measureAt(s: Double): Int = maxOf(0, lastAtOrBefore(perf.size, s) { perf[it].sec })

    fun wordAt(s: Double): Int = lastAtOrBefore(words.size, s) { words[it].s }

    /** Index of the last item starting at or before s, or -1 (binary search; items are in time order). */
    private inline fun lastAtOrBefore(size: Int, s: Double, start: (Int) -> Double): Int {
        var lo = 0
        var hi = size - 1
        var found = -1
        while (lo <= hi) {
            val mid = (lo + hi) ushr 1
            if (start(mid) <= s) { found = mid; lo = mid + 1 } else hi = mid - 1
        }
        return found
    }

    companion object {
        fun build(score: Score, range: IntRange, withCountIn: Boolean, withClicks: Boolean, only: Line? = null): Plan {
            val ms = score.measures
            val order = if (score.opts.repeats && score.hasRepeats) score.order else ms.indices.toList()
            var k0 = order.indexOf(range.first)
            if (k0 < 0) k0 = max(0, order.indexOfFirst { it >= range.first })
            var k1 = -1
            for (k in k0 until order.size) if (order[k] == range.last) k1 = k
            if (k1 < 0) {
                k1 = k0
                for (k in k0 until order.size) if (order[k] <= range.last) k1 = k
            }

            val clicks = ArrayList<Click>()
            var sec = 0.0
            val first = ms[order[k0]]
            if (withCountIn) {
                val bq = score.beatQn(first)
                val n = max(1, (first.beats * (4.0 / first.beatType) / bq).roundToInt())
                val beatSec = score.secBetween(first.t, first.t + bq).let { if (it > 0) it else 60.0 / score.opts.tempo }
                for (i in 0 until n) clicks += Click(i * beatSec, i == 0)
                sec = n * beatSec
            }
            val countIn = sec
            val perf = ArrayList<PerfMeasure>()
            for (k in k0..k1) {
                val mi = order[k]
                val meas = ms[mi]
                val dur = score.secBetween(meas.t, meas.t + meas.d)
                perf += PerfMeasure(mi, sec, dur)
                if (withClicks) {
                    val bq = score.beatQn(meas)
                    val n = max(1, (meas.d / bq).roundToInt())
                    for (j in 0 until n) clicks += Click(sec + score.secBetween(meas.t, meas.t + j * bq), j == 0)
                }
                sec += dur
            }
            val events = ArrayList<PlanEvent>()
            val lines = if (only != null) listOf(only) else score.lines
            for (line in lines) {
                for (p in perf) {
                    val meas = ms[p.mi]
                    for (e in line.byMeasure[p.mi].orEmpty()) {
                        events += PlanEvent(p.sec + score.secBetween(meas.t, e.t), score.secBetween(e.t, e.t + e.d), e.m, line)
                    }
                }
            }
            events.sortBy { it.s }
            clicks.sortBy { it.s }
            val words = ArrayList<TimedWord>()
            score.verses.getOrNull(score.opts.verse)?.let { v ->
                val byMeasure = v.words.groupBy { it.mi }
                for (p in perf) {
                    val meas = ms[p.mi]
                    for (w in byMeasure[p.mi].orEmpty()) words += TimedWord(p.sec + score.secBetween(meas.t, w.t), w.text)
                }
                words.sortBy { it.s }
            }
            return Plan(perf, events, clicks, words, sec, countIn, range)
        }

        /** The first note of each chosen line, lowest first, then all together. */
        fun startingNotes(score: Score, lines: List<Line>): Pair<Plan, String>? {
            val range = score.range()
            val firsts = lines.mapNotNull { l ->
                var found: NoteEvent? = null
                var mi = range.first
                while (found == null && mi < score.measures.size) {
                    found = l.byMeasure[mi]?.firstOrNull()
                    mi++
                }
                found?.let { l to it }
            }.sortedBy { (l, e) -> e.m + l.shift }
            if (firsts.isEmpty()) return null
            val events = ArrayList<PlanEvent>()
            firsts.forEachIndexed { i, (l, e) -> events += PlanEvent(i * 1.1, 0.95, e.m, l, 0.8f) }
            val together = firsts.size * 1.1
            for ((l, e) in firsts) events += PlanEvent(together, 1.8, e.m, l, 0.6f)
            val desc = firsts.joinToString("; ") { (l, e) -> "${l.displayName}: ${Score.noteName(e, l.shift)}" }
            return Plan(emptyList(), events, emptyList(), emptyList(), together + 1.8, 0.0, range) to desc
        }
    }
}
