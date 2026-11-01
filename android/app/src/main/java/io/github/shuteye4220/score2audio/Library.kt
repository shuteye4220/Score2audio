package io.github.shuteye4220.score2audio

import android.content.Context
import android.net.Uri
import android.provider.OpenableColumns
import org.json.JSONException
import org.json.JSONObject
import java.io.File

/** The scores kept on this device, each in its own folder. */
object Library {
    class Entry(val id: String, val title: String, val measures: Int, val added: Long, val sourceFile: String)

    private fun root(ctx: Context) = File(ctx.filesDir, "scores").apply { mkdirs() }
    private fun dir(ctx: Context, id: String) = File(root(ctx), id)

    /** The score that is open; screens share it. */
    @Volatile var current: Score? = null

    fun list(ctx: Context): List<Entry> =
        root(ctx).listFiles()?.mapNotNull { d ->
            try {
                val m = JSONObject(File(d, "meta.json").readText())
                Entry(d.name, m.getString("title"), m.optInt("measures"), m.optLong("added"), m.optString("sourceFile"))
            } catch (e: Exception) {
                null
            }
        }?.sortedBy { it.title.lowercase() }.orEmpty()

    fun displayName(ctx: Context, uri: Uri): String {
        try {
            ctx.contentResolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use { c ->
                if (c.moveToFirst()) {
                    val i = c.getColumnIndex(OpenableColumns.DISPLAY_NAME)
                    if (i >= 0) c.getString(i)?.let { return it }
                }
            }
        } catch (e: Exception) { /* fall back below */ }
        return uri.lastPathSegment?.substringAfterLast('/') ?: "score"
    }

    /** Adds a .s2a file to the library and returns its id. */
    fun import(ctx: Context, uri: Uri): String {
        refusePrivateFile(ctx, uri)
        val name = displayName(ctx, uri)
        val lower = name.lowercase()
        if (lower.endsWith(".pdf")) {
            throw ScoreFileException("PDF files are read by score2audio on a computer. Read the PDF there, choose Export for mobile, and open the .s2a file here.")
        }
        if (lower.endsWith(".mxl") || lower.endsWith(".musicxml") || lower.endsWith(".xml")) {
            throw ScoreFileException("MusicXML files cannot be opened on this device yet. Open the file in score2audio on a computer, choose Export for mobile, and open the .s2a file here.")
        }
        // A zip's table of contents is at its end, so the file is copied into
        // this app's private cache first, read, and deleted again.
        removeStaleImports(ctx)
        val tmp = File.createTempFile("import", ".s2a", ctx.cacheDir)
        val json = try {
            val input = ctx.contentResolver.openInputStream(uri) ?: throw ScoreFileException("The file could not be opened.")
            input.use { ScoreFile.copyLimited(it, tmp) }
            ScoreFile.readScoreJson(tmp)
        } finally {
            tmp.delete()
        }
        val id = "s" + System.currentTimeMillis().toString(36) + (100..999).random()
        val root = try {
            JSONObject(json)
        } catch (e: JSONException) {
            throw ScoreFileException("That score file is damaged: its score.json could not be read (${e.message}).")
        }
        val score = Score.fromJson(id, root) // checks the file before it is kept
        val d = dir(ctx, id).apply { mkdirs() }
        File(d, "score.json").writeText(json)
        root.optJSONObject("prefs")?.let { File(d, "prefs.json").writeText(it.toString()) }
        File(d, "meta.json").writeText(
            JSONObject().put("title", score.title).put("measures", score.measures.size)
                .put("added", System.currentTimeMillis()).put("sourceFile", score.sourceFile.ifBlank { name }).toString()
        )
        return id
    }

    /**
     * Another app could ask us to "open" a file:// link that points inside
     * this app's own private storage. Opening it would copy our private files
     * into the library, so such links are refused. (Files shared the normal
     * way, through content:// links, are not affected.)
     */
    private fun refusePrivateFile(ctx: Context, uri: Uri) {
        if (uri.scheme != "file") return
        val path = try {
            File(uri.path ?: "").canonicalPath
        } catch (e: Exception) {
            throw ScoreFileException("The file could not be opened.")
        }
        val own = listOfNotNull(ctx.applicationInfo.dataDir, ctx.filesDir.parent, ctx.dataDir.path)
            .mapNotNull { runCatching { File(it).canonicalPath }.getOrNull() }
        if (own.any { path == it || path.startsWith(it + File.separator) }) {
            throw ScoreFileException("The file could not be opened.")
        }
    }

    /** Copies left behind if the app was closed while adding a score. */
    private fun removeStaleImports(ctx: Context) {
        val hourAgo = System.currentTimeMillis() - 60 * 60 * 1000
        ctx.cacheDir.listFiles { f -> f.name.startsWith("import") && f.name.endsWith(".s2a") && f.lastModified() < hourAgo }
            ?.forEach { it.delete() }
    }

    fun load(ctx: Context, id: String): Score {
        current?.let { if (it.id == id) return it }
        val d = dir(ctx, id)
        val score = Score.fromJson(id, JSONObject(File(d, "score.json").readText()))
        val p = File(d, "prefs.json")
        if (p.exists()) {
            try {
                score.applyPrefs(JSONObject(p.readText()))
            } catch (e: Exception) { /* keep defaults */ }
        }
        current = score
        return score
    }

    fun savePrefs(ctx: Context, score: Score) {
        try {
            File(dir(ctx, score.id), "prefs.json").writeText(score.prefsJson().toString())
        } catch (e: Exception) { /* not important enough to stop the user */ }
    }

    fun delete(ctx: Context, id: String) {
        if (current?.id == id) current = null
        dir(ctx, id).deleteRecursively()
    }
}
