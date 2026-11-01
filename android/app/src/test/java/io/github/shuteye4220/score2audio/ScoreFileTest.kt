package io.github.shuteye4220.score2audio

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.ByteArrayInputStream
import java.io.File
import java.util.zip.ZipEntry
import java.util.zip.ZipOutputStream

class ScoreFileTest {
    @get:Rule
    val tmp = TemporaryFolder()

    private fun zipOf(vararg entries: Pair<String, ByteArray>): File {
        val f = tmp.newFile()
        ZipOutputStream(f.outputStream()).use { z ->
            for ((name, data) in entries) {
                z.putNextEntry(ZipEntry(name))
                z.write(data)
                z.closeEntry()
            }
        }
        return f
    }

    private fun expectRefused(f: File, limit: Long = ScoreFile.MAX_JSON_BYTES): String {
        try {
            ScoreFile.readScoreJson(f, limit)
        } catch (e: ScoreFileException) {
            return e.message ?: ""
        }
        fail("the file was accepted")
        return ""
    }

    @Test
    fun readsTheLayoutScore2audio100Exported() {
        // MusicXML stored uncompressed with a data descriptor, then score.json:
        // the files ZipInputStream refused.
        val f = tmp.newFile()
        javaClass.getResourceAsStream("/s2a-1.0.0-layout.s2a")!!.use { input -> f.outputStream().use { input.copyTo(it) } }
        val json = ScoreFile.readScoreJson(f)
        assertTrue(json, json.contains("\"title\":\"Layout test\""))
    }

    @Test
    fun readsTheCurrentLayout() {
        val f = zipOf("score.json" to "{\"format\":\"score2audio-score\"}".toByteArray(), "musicxml/score.mxl" to ByteArray(100))
        assertEquals("{\"format\":\"score2audio-score\"}", ScoreFile.readScoreJson(f))
    }

    @Test
    fun refusesFilesThatAreNotZips() {
        val f = tmp.newFile().apply { writeText("%PDF-1.7 not a score") }
        assertTrue(expectRefused(f).startsWith("That is not a score2audio score file."))
    }

    @Test
    fun refusesZipsWithoutScoreJson() {
        val f = zipOf("musicxml/score.mxl" to ByteArray(10))
        assertTrue(expectRefused(f).contains("no score.json"))
    }

    @Test
    fun refusesOversizedScoreJson() {
        val f = zipOf("score.json" to ByteArray(5000) { 'x'.code.toByte() })
        assertTrue(expectRefused(f, limit = 1000).contains("too large"))
    }

    @Test
    fun copyStopsAtTheLimit() {
        val dest = tmp.newFile()
        try {
            ScoreFile.copyLimited(ByteArrayInputStream(ByteArray(5000)), dest, limit = 1000)
            fail("an oversized file was copied")
        } catch (e: ScoreFileException) {
            assertTrue(dest.length() <= 1000)
        }
        ScoreFile.copyLimited(ByteArrayInputStream(ByteArray(500)), dest, limit = 1000)
        assertEquals(500L, dest.length())
    }
}
