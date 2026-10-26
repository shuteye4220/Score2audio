package io.github.shuteye4220.score2audio

import android.app.Activity
import android.os.Bundle
import android.widget.TextView

/** The lyrics, one line of music at a time; each plays from its measure. */
class LyricsActivity : Activity() {
    private lateinit var score: Score
    private lateinit var status: TextView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        score = openScore() ?: return
        title = "Lyrics"
        build()
    }

    private fun build() {
        val ui = Ui(this)
        status = ui.status()
        if (score.verses.size > 1) {
            ui.choice("Lyric line to show", score.verses.map { it.name }, score.opts.verse) {
                score.opts.verse = it
                Library.savePrefs(this, score)
                build()
            }
        }
        ui.text(
            "Lyrics are read from the printed page, so some words may be missing or misspelled. " +
                "Each item is one line of music; choose it to play from its first measure."
        )
        ui.button("Stop") { Player.stop(); status.say("Stopped.") }
        val v = score.verses.getOrNull(score.opts.verse)
        if (v == null) {
            ui.text("No lyrics were found in this score.")
        } else {
            for (ln in v.lines) {
                val a = score.measures[ln.from].n
                val b = score.measures[ln.to].n
                val where = if (a == b) "Measure $a" else "Measures $a to $b"
                ui.button("$where: ${ln.text}") {
                    score.opts.from = a
                    Library.savePrefs(this, score)
                    Player.stop()
                    if (Player.play(score)) status.say("Playing from measure $a.")
                    else status.say("No lines are chosen. Choose lines to play first.")
                }
            }
        }
        ui.show()
    }
}

/** The notes of one line, a measure at a time; each plays that measure. */
class NotesActivity : Activity() {
    private lateinit var score: Score
    private lateinit var status: TextView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        score = openScore() ?: return
        title = "Read the notes"
        if (score.lines.none { it.id == score.opts.notesLine }) {
            score.opts.notesLine = (score.lines.firstOrNull { it.enabled } ?: score.lines.firstOrNull())?.id ?: ""
        }
        build()
    }

    private fun build() {
        val ui = Ui(this)
        status = ui.status()
        val lines = score.lines
        if (lines.isEmpty()) {
            ui.text("This score has no notes.")
            ui.show()
            return
        }
        val idx = lines.indexOfFirst { it.id == score.opts.notesLine }.coerceAtLeast(0)
        ui.choice("Line to read", lines.map { it.displayName }, idx) {
            score.opts.notesLine = lines[it].id
            Library.savePrefs(this, score)
            build()
        }
        val line = lines[idx]
        ui.text(
            "Each item is one measure. Choose it to hear that measure of this line on its own." +
                if (line.shift != 0) " Notes are given at sounding pitch." else ""
        )
        for (mi in score.measures.indices) {
            val m = score.measures[mi]
            val words = score.wordsIn(mi, line)
            val text = "Measure ${m.n}: ${score.measureText(line, mi)}" + if (words.isNotEmpty()) ". Words: $words" else ""
            ui.button(text) {
                if (!Player.playMeasure(score, line, mi)) status.say("Measure ${m.n} is a rest for this line.")
            }
        }
        ui.show()
    }
}
