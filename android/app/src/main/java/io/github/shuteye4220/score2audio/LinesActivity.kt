package io.github.shuteye4220.score2audio

import android.app.Activity
import android.content.Intent
import android.os.Bundle
import android.widget.TextView

/** Check the lines to play. Each line has its own settings screen. */
class LinesActivity : Activity() {
    private lateinit var score: Score
    private lateinit var status: TextView
    private var pending = ""

    private val groups = listOf(
        Triple("voice", "Voices", "voices"),
        Triple("instrument", "Instruments", "instruments"),
        Triple("other", "Other lines", "other lines"),
        Triple("keyboard", "Keyboards", "keyboard lines"),
    )

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        score = openScore() ?: return
        title = "Lines to play"
    }

    override fun onResume() {
        super.onResume()
        if (::score.isInitialized) build()
    }

    override fun onPause() {
        super.onPause()
        if (::score.isInitialized) Library.savePrefs(this, score)
    }

    private fun build() {
        val ui = Ui(this)
        status = ui.status()
        ui.text(
            "Check each line you want to hear. Lines are named from the labels printed on the score. " +
                "When a staff holds two parts, it is offered as its upper notes, its lower notes, and all its notes. " +
                "Changes take effect while the music plays."
        )
        ui.button("Choose every line") {
            for (l in score.lines) l.enabled = l.primary
            pending = "Every line chosen."
            build()
        }
        ui.button("Choose no lines") {
            for (l in score.lines) l.enabled = false
            pending = "No lines chosen."
            build()
        }
        val present = groups.filter { (k) -> score.lines.any { it.kind == k } }
        for ((kind, heading, plural) in present) {
            ui.heading(heading)
            if (present.size > 1) {
                ui.button("Choose only the $plural") {
                    for (l in score.lines) l.enabled = l.kind == kind && l.primary
                    pending = "Only the $plural are chosen."
                    build()
                }
            }
            for (l in score.lines.filter { it.kind == kind }) {
                val label = l.displayName + (if (l.label.isNotBlank()) " (${l.name})" else "") + ", " + l.details()
                ui.check(label, l.enabled) { l.enabled = it }
                ui.button("Settings for ${l.displayName}") {
                    startActivity(Intent(this, LineSettingsActivity::class.java).putExtra("id", score.id).putExtra("line", l.id))
                }
            }
        }
        ui.show()
        if (pending.isNotEmpty()) { status.say(pending); pending = "" }
    }
}

/** Volume, sound, pitch and name of one line. */
class LineSettingsActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val score = openScore() ?: return
        val line = score.lines.firstOrNull { it.id == intent.getStringExtra("line") } ?: return finish()
        title = line.displayName
        val ui = Ui(this)
        val status = ui.status()
        ui.text(line.details())
        ui.check("Play this line", line.enabled) { line.enabled = it }

        val volumes = (0..10).map { it * 10 }
        ui.choice("Volume", volumes.map { "$it percent" }, volumes.indexOfFirst { it >= line.volume }.coerceAtLeast(0)) {
            line.volume = volumes[it]
        }
        ui.choice("Sound", Sounds.names, Sounds.ids.indexOf(line.sound).coerceAtLeast(0)) {
            line.sound = Sounds.ids[it]
        }
        val pitch = mutableListOf(
            0 to "As printed", -12 to "One octave lower", 12 to "One octave higher",
            -24 to "Two octaves lower", 24 to "Two octaves higher",
            -2 to "B-flat instrument: a whole step lower", -3 to "A instrument: a minor third lower",
            -5 to "G instrument: a fourth lower", -7 to "F instrument: a fifth lower",
            -9 to "E-flat alto instrument: a major sixth lower", 3 to "E-flat instrument: a minor third higher",
            2 to "D instrument: a whole step higher", -14 to "B-flat tenor instrument: an octave and a whole step lower",
            -21 to "E-flat baritone instrument: an octave and a major sixth lower",
        )
        if (pitch.none { it.first == line.shift }) {
            pitch += line.shift to "${if (line.shift > 0) "Up" else "Down"} ${kotlin.math.abs(line.shift)} half steps"
        }
        val pitchNames = pitch.map { (v, t) -> if (v == line.transpose && v != 0) "$t (suggested by the label)" else t }
        ui.choice("Pitch", pitchNames, pitch.indexOfFirst { it.first == line.shift }) {
            line.shift = pitch[it].first
        }

        ui.heading("Name")
        val nameField = ui.field("Your name for this line, for example Bass or First violin", line.label, false) {}
        ui.button("Save name") {
            line.label = nameField.text.toString().trim()
            Library.savePrefs(this, score)
            title = line.displayName
            status.say(if (line.label.isEmpty()) "Name set back to ${line.name}." else "Line renamed to ${line.label}.")
        }
        ui.text("The name read from the score is: ${line.name}")
        ui.button("Done") { finish() }
        ui.show()
    }

    override fun onPause() {
        super.onPause()
        Library.current?.let { Library.savePrefs(this, it) }
    }
}
