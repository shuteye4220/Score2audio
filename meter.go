package main

// Working out how long each measure really is. Audiveris sometimes misses a
// time signature (most often a change of meter partway through) or misreads
// it; the notes then keep showing the true length, so score2audio follows
// the notes where they consistently disagree with the time signature.

import "math"

type measureShape struct {
	length       float64 // quarter notes
	beats        int
	beatType     int
	meterChanged bool // the meter was worked out from the notes
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

// contentLength is how long a measure's music says it is, in quarter
// notes; 0 when it has no notes or rests.
//
// Audiveris often misses rests, so a voice that comes up short is weak
// evidence: it may just be missing something. A voice that exactly fills the
// time signature is strong evidence, so the time signature stands whenever
// any voice fills it. Otherwise the measure is as long as its longest voice.
func contentLength(lens []float64, nominal float64) float64 {
	longest := 0.0
	for _, l := range lens {
		k := math.Round(l*48) / 48
		if near(k, nominal) {
			return nominal
		}
		longest = math.Max(longest, k)
	}
	return longest
}

// agreeing counts the voices whose music lasts length c.
func agreeing(lens []float64, c float64) int {
	n := 0
	for _, l := range lens {
		if near(math.Round(l*48)/48, c) {
			n++
		}
	}
	return n
}

// meterFor writes a measure length as a time signature, keeping to the
// kind of meter the score was in where that fits.
func meterFor(q float64, prevType int) (int, int) {
	switch {
	case (prevType == 2 || q >= 7) && q >= 2 && near(q/2, math.Round(q/2)):
		// 8/4 and 10/4 are hardly ever written; 4/2 and 5/2 are
		return int(math.Round(q / 2)), 2
	case q >= 1 && near(q, math.Round(q)):
		return int(math.Round(q)), 4
	case near(q*2, math.Round(q*2)):
		return int(math.Round(q * 2)), 8
	default:
		return max(1, int(math.Round(q*4))), 16
	}
}

// inferMeasures decides each measure's length and meter.
//
// The time signature read from the score is trusted while the music agrees
// with it. Where Audiveris missed or misread one, the notes keep showing the
// true length: measures in a row whose music agrees on another length are a
// change of meter (two in a row when several voices agree, three for a
// single voice, since one voice can be misread twice). A meter learned that
// way stays in force, like a printed one, until the next printed time
// signature.
//
// A learned meter gives way as soon as the music fills the printed time
// signature again (a few genuinely short measures, such as at a section
// break, are not a new meter).
//
// A short first or last measure is a pickup or the measure that completes
// it. A repeat sign partway through a measure makes Audiveris split it into
// two short measures, the second marked incomplete, that add up to one: both
// keep their own length. A single longer measure whose several voices all
// fill the same whole number of eighths is a one-measure change of meter
// (5/8 among 2/4s). Any other single measure that disagrees keeps the meter's
// length: it is far more often a misread note or a missed rest, and keeping
// it keeps every later measure on time.
// splitPartner tells whether measure i (c long) and a neighbour marked
// incomplete together make up one measure of the given length.
func splitPartner(f []measureFacts, i int, c, length float64) bool {
	for _, j := range []int{i - 1, i + 1} {
		if j < 0 || j >= len(f) || !(f[i].implicit || f[j].implicit) {
			continue
		}
		if cj := contentLength(f[j].voiceLens, length); cj > 0 && near(c+cj, length) {
			return true
		}
	}
	return false
}

func inferMeasures(f []measureFacts) []measureShape {
	n := len(f)
	out := make([]measureShape, n)
	learned := measureShape{} // a meter worked out from the notes, while it lasts
	for i := 0; i < n; {
		b, bt := f[i].beats, f[i].beatType
		if b <= 0 || bt <= 0 {
			b, bt = 4, 4
		}
		if f[i].hasTime {
			learned = measureShape{} // a printed time signature takes over again
		}
		printed := measureShape{length: float64(b) * 4 / float64(bt), beats: b, beatType: bt}
		if learned.length > 0 && near(contentLength(f[i].voiceLens, printed.length), printed.length) {
			learned = measureShape{} // the music fits the printed time signature again
		}
		meter := printed
		if learned.length > 0 {
			meter = learned
		}
		out[i] = meter
		c := contentLength(f[i].voiceLens, meter.length)
		if c == 0 || near(c, meter.length) {
			i++
			continue
		}
		// how many measures in a row agree on c, and how strongly
		j, strong := i, agreeing(f[i].voiceLens, c) >= 2
		for j+1 < n && !f[j+1].hasTime && near(contentLength(f[j+1].voiceLens, meter.length), c) {
			j++
			strong = strong && agreeing(f[j].voiceLens, c) >= 2
		}
		if run := j - i + 1; run >= 3 || (run == 2 && strong) {
			nb, nbt := meterFor(c, meter.beatType)
			learned = measureShape{length: c, beats: nb, beatType: nbt, meterChanged: true}
			for k := i; k <= j; k++ {
				out[k] = learned
			}
			i = j + 1
			continue
		}
		switch {
		case (i == 0 || i == n-1) && c < meter.length:
			out[i].length = c // a pickup, or the measure that completes it
		case c < meter.length && splitPartner(f, i, c, meter.length):
			out[i].length = c // half of a measure split by a repeat sign
		case c > meter.length && near(c*2, math.Round(c*2)) && len(f[i].voiceLens) >= 2 && agreeing(f[i].voiceLens, c) == len(f[i].voiceLens):
			// a single measure in another meter (5/8 among 2/4s, say) whose
			// time signature was missed: every voice fills the same whole
			// number of eighths
			nb, nbt := meterFor(c, meter.beatType)
			out[i] = measureShape{length: c, beats: nb, beatType: nbt, meterChanged: true}
		}
		i++
	}
	return out
}
