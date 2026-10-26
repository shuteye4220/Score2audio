package io.github.shuteye4220.score2audio

import android.app.Activity
import android.content.Intent
import android.net.Uri
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.widget.TextView

/** The list of scores on this device, and adding new ones. */
class MainActivity : Activity() {
    private lateinit var status: TextView
    private var pendingMessage = ""
    private val main = Handler(Looper.getMainLooper())

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        title = getString(R.string.app_name)
        build()
        if (savedInstanceState == null) handleIntent(intent)
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleIntent(intent)
    }

    override fun onResume() {
        super.onResume()
        build()
    }

    private fun build() {
        val ui = Ui(this)
        status = ui.status()
        ui.heading("Your scores")
        val entries = Library.list(this)
        if (entries.isEmpty()) {
            ui.text("There are no scores on this device yet.")
        }
        for (e in entries) {
            ui.button("${e.title}, ${e.measures} measures") { open(e.id) }
        }
        ui.heading("Add a score")
        ui.button("Add a score file") { pick() }
        ui.text(
            "Scores are made with score2audio on a computer. Read the PDF there, choose Export for mobile, " +
                "and copy the file ending in .s2a to this device. Then choose Add a score file, or open the .s2a file " +
                "from your file manager."
        )
        ui.show()
        if (pendingMessage.isNotEmpty()) {
            status.say(pendingMessage)
            pendingMessage = ""
        }
    }

    private fun open(id: String) {
        startActivity(Intent(this, ScoreActivity::class.java).putExtra("id", id))
    }

    private fun pick() {
        val i = Intent(Intent.ACTION_OPEN_DOCUMENT)
            .addCategory(Intent.CATEGORY_OPENABLE)
            .setType("*/*")
        try {
            startActivityForResult(i, PICK)
        } catch (e: Exception) {
            status.say("This device has no file picker that score2audio can use. Open the .s2a file from your file manager instead.")
        }
    }

    @Deprecated("Uses the platform activity result API")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode == PICK && resultCode == RESULT_OK) data?.data?.let { importFile(it) }
    }

    private fun handleIntent(intent: Intent?) {
        val uri: Uri? = when (intent?.action) {
            Intent.ACTION_VIEW -> intent.data
            Intent.ACTION_SEND -> {
                @Suppress("DEPRECATION")
                intent.getParcelableExtra(Intent.EXTRA_STREAM)
            }
            else -> null
        }
        uri?.let { importFile(it) }
    }

    private fun importFile(uri: Uri) {
        status.say("Adding the score.")
        Thread {
            try {
                val id = Library.import(this, uri)
                val title = Library.list(this).firstOrNull { it.id == id }?.title ?: "the score"
                main.post {
                    pendingMessage = "Added $title."
                    build()
                    open(id)
                }
            } catch (e: ScoreFileException) {
                main.post { status.say(e.message ?: "That file could not be added.") }
            } catch (e: Exception) {
                main.post { status.say("That file could not be added: ${e.message ?: e}") }
            }
        }.start()
    }

    companion object {
        private const val PICK = 1
    }
}
