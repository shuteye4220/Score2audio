package io.github.shuteye4220.score2audio

import java.io.ByteArrayOutputStream
import java.io.File
import java.io.IOException
import java.io.InputStream
import java.util.zip.ZipFile

/**
 * Reading score.json out of a score file (.s2a). Plain Java and Kotlin only,
 * so it can be tested without an Android device.
 */
object ScoreFile {
    /** Largest score file accepted (the whole .s2a, as copied from the device's storage). */
    const val MAX_FILE_BYTES = 200L * 1024 * 1024

    /**
     * Largest score.json accepted, counted as it is unpacked. An 11-page
     * choral score is about 0.2 MB; this leaves room for very long scores
     * while staying parseable in a notetaker's memory.
     */
    const val MAX_JSON_BYTES = 32L * 1024 * 1024

    const val NOT_A_SCORE = "That is not a score2audio score file. Score files end in .s2a and are made with " +
        "Export for mobile in score2audio on a computer."

    /** Copies [input] to [dest], refusing anything larger than [limit] bytes. */
    fun copyLimited(input: InputStream, dest: File, limit: Long = MAX_FILE_BYTES) {
        dest.outputStream().use { out ->
            val buf = ByteArray(64 * 1024)
            var total = 0L
            while (true) {
                val n = input.read(buf)
                if (n < 0) break
                total += n
                if (total > limit) throw ScoreFileException("That file is too large to be a score file.")
                out.write(buf, 0, n)
            }
        }
    }

    /**
     * Returns the text of score.json inside [file].
     *
     * The file is opened through the zip's table of contents (ZipFile), not
     * read as a stream (ZipInputStream). Score files from score2audio 1.0.0
     * store the MusicXML uncompressed, with its size written after the data;
     * that is valid, but ZipInputStream refuses it ("only DEFLATED entries can
     * have EXT descriptor"). Reading the table of contents works for every
     * layout, and only score.json is unpacked.
     */
    fun readScoreJson(file: File, limit: Long = MAX_JSON_BYTES): String {
        val zip = try {
            ZipFile(file)
        } catch (e: IOException) {
            throw ScoreFileException("$NOT_A_SCORE (${e.message ?: "it is not a zip file"})")
        }
        return zip.use { z ->
            val entry = z.getEntry("score.json")
                ?: throw ScoreFileException("$NOT_A_SCORE (It has no score.json inside.)")
            // The size in the table of contents can be wrong, so count what is
            // actually unpacked.
            if (entry.size > limit) throw ScoreFileException("That score file is too large.")
            // Start small whatever size the file claims, and grow as data arrives.
            val out = ByteArrayOutputStream(64 * 1024)
            try {
                z.getInputStream(entry).use { input ->
                    val buf = ByteArray(64 * 1024)
                    var total = 0L
                    while (true) {
                        val n = input.read(buf)
                        if (n < 0) break
                        total += n
                        if (total > limit) throw ScoreFileException("That score file is too large.")
                        out.write(buf, 0, n)
                    }
                }
            } catch (e: IOException) {
                throw ScoreFileException("That score file is damaged (${e.message ?: e.javaClass.simpleName}).")
            }
            out.toString("UTF-8")
        }
    }
}
