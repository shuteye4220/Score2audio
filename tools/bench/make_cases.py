"""Builds the timing bench: for each case, the source MusicXML, the exact
timing truth, and a PDF engraved in that case's style.

usage: python3 make_cases.py OUTDIR
"""
import json, os, sys, warnings, io
warnings.filterwarnings('ignore')
from music21 import corpus, converter, stream, tempo, duration, meter, note, chord

# id, corpus path, (first, last) measures or None, tempo mark to print, engraving font, scanned look
# tempo marks: ('mm', beat quarter-length, number) or ('words', text) or None to keep what the source has
CASES = [
    ('reel-cut-time-half-note-mark', 'airdsAirs/book1.abc', None, None, 'Leipzig', False),
    ('cut-time-quarter-note-mark', 'airdsAirs/book6.abc', None, None, 'Bravura', False),
    ('two-four-quarter-mark', 'airdsAirs/book2.abc', None, None, 'Leland', False),
    ('meter-changes-2-4-and-5-8', 'essenFolksong/variant0.abc', None, ('words', 'Moderato'), 'Leipzig', False),
    ('three-eight-pickup-dotted-mark', 'essenFolksong/ballad50.abc', None, ('mm', 1.5, 50), 'Petaluma', False),
    ('six-eight-hymn-four-parts', 'johnson_j_r/lift_every_voice.mxl', (0, 12), ('mm', 1.5, 54), 'Leipzig', False),
    ('handel-four-four-to-three-four', 'handel/rinaldo/Lascia_chio_pianga.mxl', (1, 20), ('words', 'Largo'), 'Bravura', False),
    ('beethoven-nine-eight-dotted-mark', 'beethoven/opus18no1/movement2.mxl', (1, 10), ('mm', 1.5, 46), 'Leland', False),
    ('beethoven-meter-and-tempo-change', 'beethoven/opus74.mxl', (255, 272), None, 'Leipzig', False),
    ('mozart-three-eight-presto', 'mozart/k156/movement1.mxl', (1, 24), ('words', 'Presto'), 'Gootville', False),
    ('mozart-cut-time-half-mark-scan', 'mozart/k80/movement4.mxl', (1, 24), ('mm', 2.0, 80), 'Leipzig', True),
    ('bach-chorale-scan', 'bach/bwv66.6', None, ('mm', 1.0, 72), 'Bravura', True),
    ('trecento-four-eight-six-eight', 'trecento/PMFC_12_16-Sanctus PMFC12.16.xml', (100, 122), None, 'Leipzig', False),
    ('gloria-six-eight-to-two-four', 'luca/gloria.xml', (22, 34), ('mm', 1.5, 60), 'Petaluma', False),
    ('gloria-six-eight-to-three-four', 'luca/gloria.xml', (104, 116), None, 'Leipzig', False),
    ('triplets-two-four', 'beethoven/opus18no1/movement4.mxl', (1, 16), ('mm', 1.0, 120), 'Leland', False),
    ('aloha-oe-seven-staves', 'liliuokalani', None, ('words', 'Slowly'), 'Leipzig', False),
    ('reel-cut-time-no-mark', 'oneills1850/1176-1275.abc', None, None, 'Bravura', False),
    ('nine-eight-dotted-mark-scan', 'beethoven/opus18no1/movement2.mxl', (11, 20), ('mm', 1.5, 46), 'Petaluma', True),
    ('four-two-no-mark', 'essenFolksong/altdeu10.abc', None, None, 'Leipzig', False),
]


def load(path):
    s = corpus.parse(path)
    if isinstance(s, stream.Opus):
        s = s.scores[0]
    return s


def set_tempo(s, mark):
    if mark is None:
        return
    for t in list(s.recurse().getElementsByClass(tempo.TempoIndication)):
        t.activeSite.remove(t)
    m1 = s.parts[0].getElementsByClass('Measure')[0]
    if mark[0] == 'mm':
        mm = tempo.MetronomeMark(number=mark[2], referent=duration.Duration(mark[1]))
        m1.insert(0, mm)
    else:
        m1.insert(0, tempo.TempoText(mark[1]))


def truth(s):
    p0 = s.parts[0]
    ms = []
    first = next(iter(p0.recurse().getElementsByClass(meter.TimeSignature)), None)
    cur = first.ratioString if first else '4/4'
    for m in p0.getElementsByClass('Measure'):
        if m.timeSignature is not None:
            cur = m.timeSignature.ratioString
        ms.append(dict(n=str(m.number), t=float(m.getOffsetInHierarchy(s)), d=float(m.duration.quarterLength), ts=cur))
    # only tempo marks actually printed, with the time they take effect
    tmap = []
    for mm in s.parts[0].recurse().getElementsByClass(tempo.MetronomeMark):
        if mm.number and not mm.numberImplicit:
            tmap.append(dict(t=float(mm.getOffsetInHierarchy(s)), qbpm=round(float(mm.getQuarterBPM()), 3)))
    tmap.sort(key=lambda x: x['t'])
    words = [t.text for t in s.recurse().getElementsByClass(tempo.TempoText)]
    notes = []
    st = s.stripTies()
    for pi, p in enumerate(st.parts):
        for n in p.recurse().notes:
            off = float(n.getOffsetInHierarchy(st))
            for pt in (n.pitches if isinstance(n, chord.Chord) else [n.pitch]):
                notes.append([round(off, 4), pt.midi, pi])
    return dict(measures=ms, tempo=tmap, words=words, notes=notes, parts=len(s.parts))


def engrave(xml_path, pdf_path, font, scan):
    import verovio, subprocess
    tk = verovio.toolkit()
    tk.setOptions({'pageWidth': 2100, 'pageHeight': 2970, 'scale': 42, 'font': font, 'footer': 'none',
                   'adjustPageHeight': False, 'breaks': 'auto'})
    tk.loadFile(xml_path)
    svgs = []
    for i in range(1, tk.getPageCount() + 1):
        svg = pdf_path + f'.p{i}.svg'
        open(svg, 'w').write(tk.renderToSVG(i))
        svgs.append(svg)
    printed = pdf_path + '.printed.pdf'
    subprocess.run(['node', os.path.join(os.path.dirname(os.path.abspath(__file__)), 'svg2pdf.js'), printed] + svgs, check=True)
    for svg in svgs:
        os.remove(svg)
    pages = [printed]
    if scan:
        import pypdfium2 as pdfium
        from PIL import Image, ImageFilter
        import random
        random.seed(7)
        images = []
        doc = pdfium.PdfDocument(printed)
        for i in range(len(doc)):
            img = doc[i].render(scale=300 / 72).to_pil().convert('L')
            img = img.rotate(random.choice([-0.4, 0.3, 0.5]), resample=Image.BICUBIC, fillcolor=255)
            img = img.filter(ImageFilter.GaussianBlur(0.8))
            px = img.load()
            for _ in range(img.width * img.height // 400):   # specks of scanner dust
                x, y = random.randrange(img.width), random.randrange(img.height)
                px[x, y] = random.choice([0, 255])
            images.append(img.point(lambda v: 255 if v > 150 else (v * 0.6)))
        images[0].save(pdf_path, save_all=True, append_images=images[1:], resolution=300)
    else:
        os.replace(printed, pdf_path)
    if os.path.exists(printed):
        os.remove(printed)


def main(outdir):
    only = set(sys.argv[2:])
    for cid, path, rng, mark, font, scan in CASES:
        if only and cid not in only:
            continue
        d = os.path.join(outdir, cid)
        os.makedirs(d, exist_ok=True)
        s = load(path)
        if rng:
            s = s.measures(rng[0], rng[1])
        set_tempo(s, mark)
        xml = os.path.join(d, 'source.musicxml')
        s.write('musicxml', xml)
        s = converter.parse(xml)   # truth from exactly what is engraved
        json.dump(truth(s), open(os.path.join(d, 'truth.json'), 'w'))
        if not os.path.exists(os.path.join(d, 'score.pdf')):
            engrave(xml, os.path.join(d, 'score.pdf'), font, scan)
        json.dump(dict(id=cid, source=path, range=rng, mark=mark, font=font, scan=scan), open(os.path.join(d, 'case.json'), 'w'))
        t = json.load(open(os.path.join(d, 'truth.json')))
        print(f"{cid:36} parts={t['parts']} measures={len(t['measures'])} meters={sorted(set(m['ts'] for m in t['measures']))} tempo={t['tempo'][:2]} words={t['words'][:1]}")


if __name__ == '__main__':
    main(sys.argv[1])
