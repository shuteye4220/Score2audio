package main

// Printed metronome marks ("♩ = 72", "𝅗𝅥 = 60", "♩. = 54", "♪ = 70").
//
// Audiveris does not recognise the note symbol before the "=", so it leaves
// tempo marks out of its MusicXML altogether, and it reads the number only
// some of the time. score2audio finds the marks in Audiveris's saved project
// (the .omr file, a zip holding each page's analysis and a black-and-white
// image of the page): an "=" above a system's top staff, a note symbol to its
// left and a number of 20 to 320 to its right. The note's head (filled or
// hollow), stem, flags and dot give the beat the number counts, which is
// what decides between a tempo and double or half that tempo. The number is
// the one Audiveris read there, or else read by comparing each digit's shape
// with the digits of common fonts (digits_gen.go).

import (
	"archive/zip"
	"encoding/xml"
	"image"
	"image/png"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
)

type omrTempo struct {
	measure int     // index among the exported measures
	unit    float64 // the beat the mark counts, in quarter notes
	number  int     // beats per minute as printed
}

type omrBounds struct {
	X float64 `xml:"x,attr"`
	Y float64 `xml:"y,attr"`
	W float64 `xml:"w,attr"`
	H float64 `xml:"h,attr"`
}

type omrSheet struct {
	Interline struct {
		Main float64 `xml:"main,attr"`
	} `xml:"scale>interline"`
	Systems []struct {
		Stacks []struct {
			Left  float64 `xml:"left,attr"`
			Right float64 `xml:"right,attr"`
		} `xml:"stack"`
		Parts []struct {
			Staves []struct {
				ID    string `xml:"id,attr"`
				Lines []struct {
					Points []struct {
						X float64 `xml:"x,attr"`
						Y float64 `xml:"y,attr"`
					} `xml:"point"`
				} `xml:"lines>line"`
			} `xml:"staff"`
		} `xml:"part"`
		Words []struct {
			Value  string    `xml:"value,attr"`
			Bounds omrBounds `xml:"bounds"`
		} `xml:"sig>inters>word"`
	} `xml:"page>system"`
}

var sheetNameRe = regexp.MustCompile(`^sheet#(\d+)/sheet#(\d+)\.xml$`)
var numberWordRe = regexp.MustCompile(`(\d{2,3})`)

const maxOMRPart = 64 << 20 // no page's analysis or image is bigger than this

// readOMRTempos lists the tempo marks in an Audiveris project, skipping the
// pages left out of the export.
func readOMRTempos(path string, skipped []int) []omrTempo {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil
	}
	defer zr.Close()
	files := map[string]*zip.File{}
	var sheets []int
	for _, f := range zr.File {
		files[f.Name] = f
		if m := sheetNameRe.FindStringSubmatch(f.Name); m != nil && m[1] == m[2] {
			n, _ := strconv.Atoi(m[1])
			sheets = append(sheets, n)
		}
	}
	sort.Ints(sheets)
	if len(sheets) > 2000 {
		return nil
	}
	var marks []omrTempo
	measure := 0
	for _, n := range sheets {
		if containsInt(skipped, n) {
			continue
		}
		name := "sheet#" + strconv.Itoa(n)
		var sh omrSheet
		if err := decodeZipXML(files[name+"/"+name+".xml"], &sh); err != nil {
			continue
		}
		img := decodeZipPNG(files[name+"/BINARY.png"])
		il := sh.Interline.Main
		if il < 6 || il > 200 {
			il = 20
		}
		for _, sys := range sh.Systems {
			if img != nil && len(sys.Parts) > 0 && len(sys.Parts[0].Staves) > 0 && len(sys.Stacks) > 0 {
				top := sys.Parts[0].Staves[0]
				if len(top.Lines) == 0 || len(top.Lines[0].Points) == 0 {
					measure += len(sys.Stacks)
					continue
				}
				topY, left, right := math.Inf(1), math.Inf(1), math.Inf(-1)
				for _, p := range top.Lines[0].Points {
					topY, left, right = math.Min(topY, p.Y), math.Min(left, p.X), math.Max(right, p.X)
				}
				var words []omrBounds
				var values []int
				for _, w := range sys.Words {
					if m := numberWordRe.FindString(w.Value); m != "" && w.Bounds.Y < topY {
						v, _ := strconv.Atoi(m)
						words, values = append(words, w.Bounds), append(values, v)
					}
				}
				for _, mk := range findMetronomeMarks(img, int(left-2*il), int(topY-7*il), int(right), int(topY-0.2*il), il, words, values) {
					k := 0
					for k+1 < len(sys.Stacks) && sys.Stacks[k].Right < mk.x {
						k++
					}
					marks = append(marks, omrTempo{measure: measure + k, unit: mk.unit, number: mk.number})
				}
			}
			measure += len(sys.Stacks)
		}
	}
	return marks
}

func decodeZipXML(f *zip.File, v interface{}) error {
	if f == nil {
		return io.ErrUnexpectedEOF
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	return xml.NewDecoder(io.LimitReader(rc, maxOMRPart)).Decode(v)
}

func decodeZipPNG(f *zip.File) image.Image {
	if f == nil {
		return nil
	}
	rc, err := f.Open()
	if err != nil {
		return nil
	}
	cfg, err := png.DecodeConfig(io.LimitReader(rc, maxOMRPart))
	rc.Close()
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > 80<<20 {
		return nil // not a page image of any sensible size
	}
	rc, err = f.Open()
	if err != nil {
		return nil
	}
	defer rc.Close()
	img, err := png.Decode(io.LimitReader(rc, maxOMRPart))
	if err != nil {
		return nil
	}
	return img
}

// ---------- reading the note symbol ----------

type blob struct {
	minX, minY, maxX, maxY, area int
	pix                          []image.Point
}

func (b blob) w() int { return b.maxX - b.minX + 1 }
func (b blob) h() int { return b.maxY - b.minY + 1 }

// inkMap is the black pixels of a window of the page, with long horizontal
// runs (staff lines, ledger lines) removed so notes do not join them.
type inkMap struct {
	x0, y0, w, h int
	ink          []bool
}

func newInkMap(img image.Image, x0, y0, x1, y1 int, il float64) *inkMap {
	r := img.Bounds()
	x0, y0 = max(x0, r.Min.X), max(y0, r.Min.Y)
	x1, y1 = min(x1, r.Max.X), min(y1, r.Max.Y)
	if x1 <= x0 || y1 <= y0 {
		return nil
	}
	m := &inkMap{x0: x0, y0: y0, w: x1 - x0, h: y1 - y0}
	m.ink = make([]bool, m.w*m.h)
	gray, _ := img.(*image.Gray) // what Audiveris writes; read directly, it is much faster
	for y := 0; y < m.h; y++ {
		for x := 0; x < m.w; x++ {
			if gray != nil {
				m.ink[y*m.w+x] = gray.Pix[gray.PixOffset(x0+x, y0+y)] < 0x80
				continue
			}
			g, _, _, _ := img.At(x0+x, y0+y).RGBA()
			m.ink[y*m.w+x] = g < 0x8000
		}
		// erase runs longer than a few staff spaces: lines, not symbols
		run := 0
		for x := 0; x <= m.w; x++ {
			if x < m.w && m.ink[y*m.w+x] {
				run++
				continue
			}
			if float64(run) > 2.5*il {
				for k := x - run; k < x; k++ {
					m.ink[y*m.w+k] = false
				}
			}
			run = 0
		}
	}
	return m
}

func (m *inkMap) at(x, y int) bool {
	x, y = x-m.x0, y-m.y0
	return x >= 0 && y >= 0 && x < m.w && y < m.h && m.ink[y*m.w+x]
}

// blobs finds the connected shapes in the window.
func (m *inkMap) blobs(minArea int) []blob {
	seen := make([]bool, len(m.ink))
	var out []blob
	for i, on := range m.ink {
		if !on || seen[i] {
			continue
		}
		b := blob{minX: math.MaxInt, minY: math.MaxInt, maxX: -1, maxY: -1}
		stack := []int{i}
		seen[i] = true
		for len(stack) > 0 {
			j := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			x, y := j%m.w, j/m.w
			p := image.Point{X: m.x0 + x, Y: m.y0 + y}
			b.pix = append(b.pix, p)
			b.minX, b.maxX = min(b.minX, p.X), max(b.maxX, p.X)
			b.minY, b.maxY = min(b.minY, p.Y), max(b.maxY, p.Y)
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || nx >= m.w || ny >= m.h {
						continue
					}
					k := ny*m.w + nx
					if m.ink[k] && !seen[k] {
						seen[k] = true
						stack = append(stack, k)
					}
				}
			}
		}
		b.area = len(b.pix)
		if b.area >= minArea {
			out = append(out, b)
		}
	}
	return out
}

type foundMark struct {
	x      float64 // where the note is, across the page
	unit   float64
	number int
}

func isBar(b blob, il float64) bool {
	return float64(b.h()) <= math.Max(3, 0.3*il) && float64(b.w()) >= 0.35*il && float64(b.w()) <= 2*il && b.w() >= 2*b.h()
}

// findMetronomeMarks looks for "note = number" in a part of the page. words
// and values are the numbers Audiveris read in that part of the page.
func findMetronomeMarks(img image.Image, x0, y0, x1, y1 int, il float64, words []omrBounds, values []int) []foundMark {
	m := newInkMap(img, x0, y0, x1, y1, il)
	if m == nil {
		return nil
	}
	blobs := m.blobs(max(3, int(0.01*il*il)))
	if len(blobs) > 5000 {
		return nil // not a page of music
	}
	sort.Slice(blobs, func(a, b int) bool { return blobs[a].minX < blobs[b].minX })
	var out []foundMark
	used := map[int]bool{}
	for i, a := range blobs {
		if used[i] || !isBar(a, il) {
			continue
		}
		for j := i + 1; j < len(blobs) && blobs[j].minX <= a.maxX; j++ {
			b := blobs[j]
			if used[j] || !isBar(b, il) {
				continue
			}
			upper, lower := a, b
			if upper.minY > lower.minY {
				upper, lower = lower, upper
			}
			overlap := min(upper.maxX, lower.maxX) - max(upper.minX, lower.minX)
			gap := float64(lower.minY - upper.maxY)
			wa, wb := float64(upper.w()), float64(lower.w())
			if float64(overlap) < 0.7*math.Min(wa, wb) || math.Abs(wa-wb) > 0.35*math.Max(wa, wb) || gap < 0.1*il || gap > 0.7*il {
				continue
			}
			eq := blob{minX: min(upper.minX, lower.minX), maxX: max(upper.maxX, lower.maxX), minY: upper.minY, maxY: lower.maxY}
			number, ok := readMarkNumber(blobs, eq, il, words, values)
			if !ok {
				continue
			}
			unit, x, ok := readMarkNote(blobs, eq, il)
			if !ok {
				continue
			}
			used[i], used[j] = true, true
			out = append(out, foundMark{x: x, unit: unit, number: number})
			break
		}
	}
	return out
}

// readMarkNumber reads the number right of an "=".
func readMarkNumber(blobs []blob, eq blob, il float64, words []omrBounds, values []int) (int, bool) {
	midY := float64(eq.minY+eq.maxY) / 2
	var digits []blob
	right := float64(eq.maxX)
	for _, b := range blobs {
		if float64(b.minX) <= right-0.1*il && len(digits) > 0 || b.minX <= eq.maxX {
			continue
		}
		cy := float64(b.minY+b.maxY) / 2
		if float64(b.minX)-right > 1.0*il || math.Abs(cy-midY) > 0.7*il || float64(b.h()) < 0.7*il || float64(b.h()) > 3*il {
			if len(digits) > 0 && float64(b.minX)-right > 1.0*il {
				break
			}
			continue
		}
		digits = append(digits, b)
		right = float64(b.maxX)
		if len(digits) == 3 {
			break
		}
	}
	if len(digits) < 2 {
		return 0, false
	}
	// the number Audiveris read there, if it read one
	for k, w := range words {
		if w.X < right && w.X+w.W > float64(digits[0].minX) && w.Y < float64(digits[0].maxY) && w.Y+w.H > float64(digits[0].minY) {
			if values[k] >= 20 && values[k] <= 320 {
				return values[k], true
			}
		}
	}
	n := 0
	for _, d := range digits {
		v, ok := readDigit(d)
		if !ok {
			if n >= 20 {
				break // "72-80", "54 ca.": the first number is the tempo
			}
			return 0, false
		}
		n = n*10 + v
	}
	return n, n >= 20 && n <= 320
}

// readMarkNote reads the note symbol left of an "=": the beat in quarter
// notes, and where the note is across the page.
func readMarkNote(blobs []blob, eq blob, il float64) (float64, float64, bool) {
	var note *blob
	for i := range blobs {
		b := &blobs[i]
		if b.maxX >= eq.minX || float64(b.maxX) < float64(eq.minX)-2.5*il || float64(b.h()) < 1.5*il || float64(b.h()) > 5*il ||
			float64(b.maxY) < float64(eq.minY)-0.3*il || float64(b.minY) > float64(eq.maxY) {
			continue
		}
		if note == nil || b.maxX > note.maxX {
			note = b
		}
	}
	if note == nil {
		return 0, 0, false
	}
	set := make(map[image.Point]bool, len(note.pix))
	for _, p := range note.pix {
		set[p] = true
	}

	// the stem: the longest vertical run, as wide as the columns that run
	// nearly as far (a bold stem is several pixels wide)
	colRun := func(x int) (int, int) {
		best, bottom, run := 0, 0, 0
		for y := note.minY; y <= note.maxY+1; y++ {
			if set[image.Point{X: x, Y: y}] {
				run++
				continue
			}
			if run > best {
				best, bottom = run, y-1
			}
			run = 0
		}
		return best, bottom
	}
	stemX, stemLen, stemBottom := 0, 0, 0
	for x := note.minX; x <= note.maxX; x++ {
		if l, bot := colRun(x); l > stemLen {
			stemX, stemLen, stemBottom = x, l, bot
		}
	}
	if float64(stemLen) < 1.6*il {
		return 0, 0, false // a whole note is never the beat of a tempo mark worth reading
	}
	stemLeft, stemRight := stemX, stemX
	for stemLeft > note.minX {
		if l, _ := colRun(stemLeft - 1); float64(l) < 0.8*float64(stemLen) {
			break
		}
		stemLeft--
	}
	for stemRight < note.maxX {
		if l, _ := colRun(stemRight + 1); float64(l) < 0.8*float64(stemLen) {
			break
		}
		stemRight++
	}

	// the head: where the shape reaches left of the stem (tempo marks have
	// their stems up, on the right of the head)
	hx0, hx1, hy0, hy1 := math.MaxInt, -1, math.MaxInt, -1
	for _, p := range note.pix {
		if p.X < stemLeft-1 {
			hy0, hy1 = min(hy0, p.Y), max(hy1, p.Y)
		}
	}
	if hy1 < 0 {
		return 0, 0, false
	}
	for _, p := range note.pix {
		if p.Y >= hy0 && p.Y <= hy1 {
			hx0, hx1 = min(hx0, p.X), max(hx1, p.X)
		}
	}
	headTop := float64(hy0)
	if float64(hx1-hx0) < 0.7*il || float64(hy1-hy0) < 0.4*il || stemBottom < hy0 {
		return 0, 0, false
	}
	// filled or hollow: how much of the middle of the head is ink
	cx0, cx1 := hx0+(hx1-hx0)/4, hx0+(hx1-hx0)*3/4
	cy0, cy1 := hy0+(hy1-hy0)/4, hy0+(hy1-hy0)*3/4
	in, all := 0, 0
	for y := cy0; y <= cy1; y++ {
		for x := cx0; x <= cx1; x++ {
			if x >= stemLeft-1 && x <= stemRight+1 {
				continue
			}
			all++
			if set[image.Point{X: x, Y: y}] {
				in++
			}
		}
	}
	if all == 0 {
		return 0, 0, false
	}
	unit := 1.0
	if float64(in)/float64(all) < 0.55 {
		unit = 2
	} else {
		// flags: separate stretches of ink just right of the stem, above the head
		flags, inRun := 0, false
		fx := stemRight + max(2, int(0.3*il))
		for y := note.minY; float64(y) < headTop-0.3*float64(hy1-hy0); y++ {
			on := set[image.Point{X: fx, Y: y}] || set[image.Point{X: fx + 1, Y: y}]
			if on && !inRun {
				flags++
			}
			inRun = on
		}
		unit = 1 / math.Pow(2, float64(min(flags, 3)))
	}

	// a dot: a small round shape right of the head, before the "="
	for _, b := range blobs {
		if b.minX <= note.maxX-1 || b.maxX >= eq.minX {
			continue
		}
		bw, bh := float64(b.w()), float64(b.h())
		cy := float64(b.minY+b.maxY) / 2
		if dotMax := math.Max(0.7*il, 0.6*float64(hy1-hy0)); bw >= 0.15*il && bh >= 0.15*il && bw <= dotMax && bh <= dotMax && bw/bh < 1.8 && bh/bw < 1.8 &&
			cy > float64(hy0)-0.5*il && cy < float64(hy1)+0.3*il {
			unit *= 1.5
			break
		}
	}
	return unit, float64(note.minX+note.maxX) / 2, true
}

// ---------- reading digits ----------

const digitGridW, digitGridH = 8, 12

type digitTemplate struct {
	digit  int
	aspect float64 // width / height
	grid   string  // ink in each cell, 0-f, row by row
}

// readDigit compares a shape with the digits of common fonts.
func readDigit(b blob) (int, bool) {
	w, h := b.w(), b.h()
	if w < 2 || h < 6 || w > 3*h {
		return 0, false
	}
	ink := make([]bool, w*h)
	for _, p := range b.pix {
		ink[(p.Y-b.minY)*w+p.X-b.minX] = true
	}
	var f [digitGridW * digitGridH]float64
	for gy := 0; gy < digitGridH; gy++ {
		for gx := 0; gx < digitGridW; gx++ {
			xa, xb := gx*w/digitGridW, max(gx*w/digitGridW+1, ((gx+1)*w+digitGridW-1)/digitGridW)
			ya, yb := gy*h/digitGridH, max(gy*h/digitGridH+1, ((gy+1)*h+digitGridH-1)/digitGridH)
			on, all := 0, 0
			for y := ya; y < min(yb, h); y++ {
				for x := xa; x < min(xb, w); x++ {
					all++
					if ink[y*w+x] {
						on++
					}
				}
			}
			f[gy*digitGridW+gx] = 15 * float64(on) / float64(max(1, all))
		}
	}
	aspect := float64(w) / float64(h)
	best, second, bestD := math.Inf(1), math.Inf(1), -1
	perDigit := [10]float64{}
	for i := range perDigit {
		perDigit[i] = math.Inf(1)
	}
	for _, t := range digitTemplates {
		d := 0.0
		for i := 0; i < len(t.grid) && i < len(f); i++ {
			v := float64(hexVal(t.grid[i]))
			d += (v - f[i]) * (v - f[i])
		}
		d = math.Sqrt(d/float64(len(f))) + 12*math.Abs(t.aspect-aspect)
		perDigit[t.digit] = math.Min(perDigit[t.digit], d)
	}
	for dg, d := range perDigit {
		if d < best {
			best, second, bestD = d, best, dg
		} else if d < second {
			second = d
		}
	}
	// a clear match: close to one digit, and closer to it than to any other
	return bestD, bestD >= 0 && best < 5.5 && second-best > 0.4
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	}
	return 0
}
