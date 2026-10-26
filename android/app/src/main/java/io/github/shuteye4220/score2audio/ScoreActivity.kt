package io.github.shuteye4220.score2audio

import android.app.Activity
import android.app.AlertDialog
import android.content.Intent
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.widget.Button
import android.widget.EditText
import android.widget.TextView

/** Loads the score named in the intent, or closes the screen. */
fun Activity.openScore(): Score? {
    val id = intent.getStringExtra("id") ?: return null.also { finish() }
    return try {
        Library.load(this, id)
    } catch (e: Exception) {
        finish()
        null
    }
}

/** The practice screen for one score. */
class ScoreActivity : Activity() {
    private lateinit var score: Score
    private lateinit var status: TextView
    private lateinit var position: TextView
    private lateinit var playButton: Button
    private lateinit var tempoField: EditText
    private lateinit var fromField: EditText
    private lateinit var toField: EditText
    private lateinit var linesSummary: TextView
    private val main = Handler(Looper.getMainLooper())
    private var lastK = -3
    private var lastWord = -1
    private var stoppedText = "Stopped at the beginning."

    private val tick = object : Runnable {
        override fun run() {
            update()
            main.postDelayed(this, 150)
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        score = openScore() ?: return
        title = score.title
        build()
    }

    override fun onResume() {
        super.onResume()
        if (!::score.isInitialized) return
        linesSummary.text = linesText()
        Player.onEnded = { mode ->
            when (mode) {
                Player.Mode.SCORE -> {
                    stoppedText = "Finished. Press Play to start again."
                    status.say("Finished.")
                }
                else -> stoppedText = "Stopped."
            }
            update()
        }
        main.post(tick)
    }

    override fun onPause() {
        super.onPause()
        main.removeCallbacks(tick)
        if (::score.isInitialized) Library.savePrefs(this, score)
    }

    private fun save() = Library.savePrefs(this, score)

    private fun build() {
        val o = score.opts
        val ui = Ui(this)
        status = ui.status()
        ui.text(score.summary())
        for (w in score.warnings) ui.text(w)

        ui.heading("Play")
        playButton = ui.button("Play") { playOrPause() }
        ui.button("Stop") {
            val was = Player.mode != Player.Mode.NONE || Player.canContinue
            Player.stop()
            stoppedText = "Stopped. Play starts from the From measure."
            update()
            if (was) status.say("Stopped.")
        }
        ui.button("Give starting notes") { startingNotes() }
        ui.button("Save as audio file") { saveAudio() }
        position = ui.text(stoppedText)

        ui.heading("Tempo and measures")
        tempoField = ui.field("Tempo, in beats per minute", o.tempo.toString(), true) { setTempo(it) }
        ui.text(
            "One beat is a ${score.beatName(score.beatQn0)}. " +
                if (score.tempoFromScore) "The score asks for ${score.scoreTempo} beats per minute."
                else "The score has no tempo marking that could be read."
        )
        fromField = ui.field("From measure", o.from, true) { setMeasure(it, true) }
        toField = ui.field("To measure", o.to, true) { setMeasure(it, false) }
        ui.check("Repeat the chosen measures until stopped", o.loop) { o.loop = it; save() }
        ui.check("Count in one measure before starting", o.countIn) { o.countIn = it; save() }
        ui.check("Metronome clicks while playing", o.metronome) { o.metronome = it; save(); Player.restartHere(score) }
        if (score.hasRepeats) ui.check("Follow repeat signs", o.repeats) { o.repeats = it; save(); Player.forgetPlace() }
        ui.check("Announce measure numbers while playing", o.sayMeasures) { o.sayMeasures = it; save() }
        ui.check("Announce lyrics while playing", o.sayLyrics) { o.sayLyrics = it; save() }

        ui.heading("Lines")
        linesSummary = ui.text(linesText())
        ui.button("Choose lines to play") { go(LinesActivity::class.java) }

        ui.heading("Lyrics and notes")
        if (score.verses.isNotEmpty()) ui.button("Show the lyrics") { go(LyricsActivity::class.java) }
        else ui.text("No lyrics were found in this score.")
        ui.button("Read the notes") { go(NotesActivity::class.java) }

        ui.heading("This score")
        ui.text(
            "Notes and lyrics were read automatically from the printed score" +
                (if (score.sourceFile.isNotBlank()) " (${score.sourceFile})" else "") +
                ", so check anything that sounds wrong."
        )
        ui.button("Delete this score") { confirmDelete() }
        ui.show()
    }

    private fun go(c: Class<*>) = startActivity(Intent(this, c).putExtra("id", score.id))

    private fun linesText(): String {
        val on = score.lines.filter { it.enabled }
        return if (on.isEmpty()) "No lines are chosen."
        else "${on.size} of ${score.lines.size} lines chosen: ${on.joinToString("; ") { it.displayName }}."
    }

    /** Applies the typed tempo and measures. Returns false if one is wrong. */
    private fun applyFields(): Boolean {
        return setTempo(tempoField.text.toString(), quiet = true) &&
            setMeasure(fromField.text.toString(), true) && setMeasure(toField.text.toString(), false)
    }

    private fun setTempo(v: String, quiet: Boolean = false): Boolean {
        val n = v.trim().toIntOrNull()
        if (n == null || n < 20 || n > 300) {
            status.say("Tempo must be a number from 20 to 300.")
            tempoField.setText(score.opts.tempo.toString())
            return false
        }
        if (n != score.opts.tempo) {
            score.opts.tempo = n
            save()
            Player.restartHere(score)
            if (!quiet) status.say("Tempo set to $n beats per minute.")
        }
        return true
    }

    private fun setMeasure(v: String, isFrom: Boolean): Boolean {
        val i = if (v.isBlank()) -1 else score.measureIndex(v)
        val field = if (isFrom) fromField else toField
        if (i < 0) {
            status.say("Enter a measure number from ${score.measures.first().n} to ${score.lastNumber}.")
            field.setText(if (isFrom) score.opts.from else score.opts.to)
            return false
        }
        val n = score.measures[i].n
        val old = if (isFrom) score.opts.from else score.opts.to
        if (n != old) {
            if (isFrom) score.opts.from = n else score.opts.to = n
            Player.forgetPlace()
            save()
        }
        if (field.text.toString() != n) field.setText(n)
        return true
    }

    private fun playOrPause() {
        if (Player.isPlayingScore) {
            val m = Player.pause()
            stoppedText = if (m != null) "Paused in measure $m. Press Continue to go on." else "Paused."
            update()
            status.say("Paused.")
            return
        }
        if (!applyFields()) return
        val continuing = Player.canContinue
        if (!Player.play(score)) {
            status.say("No lines are chosen. Choose lines to play first.")
            return
        }
        lastK = -3
        lastWord = -1
        if (!continuing) {
            val r = score.range()
            status.say("Playing measures ${score.measures[r.first].n} to ${score.measures[r.last].n}.")
        }
        update()
    }

    private fun startingNotes() {
        if (!applyFields()) return
        val desc = Player.startingNotes(score)
        if (desc == null) status.say("No lines are chosen, or they have no notes after that measure.")
        else status.say("Starting notes, lowest first, then together. $desc.")
        stoppedText = "Stopped."
        update()
    }

    private fun saveAudio() {
        if (!applyFields()) return
        status.say("Making the audio file. This can take a little while for a long piece.")
        Player.saveWav(this, score) { file, error ->
            if (error != null) status.say("The audio file could not be made: $error")
            else status.say("Saved the audio file in $file.")
        }
    }

    private fun confirmDelete() {
        AlertDialog.Builder(this)
            .setTitle("Delete this score?")
            .setMessage("${score.title} will be removed from this device. The score file you added it from is not affected.")
            .setPositiveButton("Delete") { _, _ ->
                Player.stop()
                Library.delete(this, score.id)
                finish()
            }
            .setNegativeButton("Cancel", null)
            .show()
    }

    /** Keeps the Play button and the position line up to date. */
    private fun update() {
        playButton.text = when {
            Player.isPlayingScore -> "Pause"
            Player.canContinue -> "Continue"
            else -> "Play"
        }
        val p = Player.plan
        if (!Player.isPlayingScore || p == null) {
            if (position.text != stoppedText) position.text = stoppedText
            lastK = -3
            return
        }
        val now = Player.now()
        if (now < p.countInSec) {
            if (lastK != -2) { lastK = -2; position.text = "Counting in." }
            return
        }
        val k = p.measureAt(now)
        if (k != lastK && k < p.perf.size) {
            lastK = k
            val n = score.measures[p.perf[k].mi].n
            position.text = "Playing measure $n of ${score.lastNumber}."
            if (score.opts.sayMeasures) position.announceForAccessibility("Measure $n")
        }
        val w = p.wordAt(now)
        if (w != lastWord && w >= 0) {
            lastWord = w
            if (score.opts.sayLyrics) position.announceForAccessibility(p.words[w].text)
        }
    }
}
