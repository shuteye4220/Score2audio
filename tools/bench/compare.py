"""Scores score2audio's reading of each case against the truth.

usage: python3 compare.py CASESDIR SCORE2AUDIO-BINARY [--json OUT]
"""
import glob, json, os, re, subprocess, sys
from bisect import bisect_right


def seconds(points, measures_end):
    """Playing time in seconds from a tempo map [(t, qbpm)] and an end time in quarter notes."""
    if not points:
        return None
    pts = sorted(points)
    total = 0.0
    for i, (t, q) in enumerate(pts):
        end = pts[i + 1][0] if i + 1 < len(pts) else measures_end
        if end > t:
            total += (end - t) * 60.0 / q
    if pts[0][0] > 0:
        total += pts[0][0] * 60.0 / pts[0][1]
    return total


def notes_from_model(m):
    out = set()
    for l in m['lines']:
        if l['part'] in ('single', 'all'):
            for e in l['events']:
                out.add((round(e['t'] * 12), e['m']))
    return out


def f1(truth, got):
    if not truth or not got:
        return 0.0
    hit = len(truth & got)
    p, r = hit / len(got), hit / len(truth)
    return 0 if hit == 0 else 2 * p * r / (p + r)


def tempo_verdict(tq, oq, from_score):
    if tq is None:
        return 'no printed mark' + ('' if not from_score else f' (found {oq:g}?)')
    if not from_score:
        return 'MISSED'
    r = oq / tq
    for name, v in (('right', 1), ('DOUBLE', 2), ('HALF', 0.5), ('x1.5', 1.5), ('x2/3', 2 / 3), ('x3', 3), ('x1/3', 1 / 3)):
        if abs(r - v) < 0.03 * v:
            return name
    return f'WRONG x{r:.2f}'


def main():
    cases_dir, binary = sys.argv[1], sys.argv[2]
    rows = []
    for d in sorted(glob.glob(os.path.join(cases_dir, '*/'))):
        cid = os.path.basename(d.rstrip('/'))
        truth = json.load(open(os.path.join(d, 'truth.json')))
        files = sorted(glob.glob(os.path.join(d, os.environ.get('OMRDIR', 'omr'), '*.mxl')), key=lambda p: [int(x) if x.isdigit() else x for x in re.split(r'(\d+)', p)])
        if not files:
            rows.append(dict(id=cid, error='Audiveris produced no MusicXML'))
            continue
        r = subprocess.run([binary, '--dump'] + files, capture_output=True, text=True)
        if r.returncode != 0:
            rows.append(dict(id=cid, error=r.stdout.strip()[:200]))
            continue
        m = json.loads(r.stdout)
        tm, om = truth['measures'], m['measures']
        n = min(len(tm), len(om))
        meter_ok = sum(1 for i in range(n) if tm[i]['ts'] == f"{om[i]['beats']}/{om[i]['beatType']}")
        # the last measure's length only changes the silence at the end
        len_ok = sum(1 for i in range(n) if abs(tm[i]['d'] - om[i]['d']) < 0.01 or i == len(tm) - 1)
        t_end = tm[-1]['t'] + tm[-1]['d']
        o_end = om[-1]['t'] + om[-1]['d']
        t_end_cmp, o_end_cmp = tm[-1]['t'], om[-1]['t']
        tq = truth['tempo'][0]['qbpm'] if truth['tempo'] else None
        oq = m['tempo'][0]['bpm']
        t_sec = seconds([(p['t'], p['qbpm']) for p in truth['tempo']], t_end)
        o_sec = seconds([(p['t'], p['bpm']) for p in m['tempo']], o_end)
        # line both up by their first note: silence before it does not matter
        t0 = min(t for t, _, _ in truth['notes'])
        tn = {(round((t - t0) * 12), p) for t, p, _ in truth['notes']}
        on = notes_from_model(m)
        if on:
            o0 = min(t for t, _ in on)
            on = {(t - o0, p) for t, p in on}
        rows.append(dict(
            id=cid, measures=f"{len(om)}/{len(tm)}", meter=f"{meter_ok}/{n}", lengths=f"{len_ok}/{n}",
            quarters=f"{o_end_cmp:g}/{t_end_cmp:g}", tempo=tempo_verdict(tq, oq, m['tempoFromScore']),
            tempo_found=oq, tempo_true=tq, changes=f"{len(m['tempo'])}/{max(1, len(truth['tempo']))}",
            seconds=(f"{o_sec:.1f}/{t_sec:.1f}" if t_sec else f"{o_sec:.1f}/-"),
            notes=round(f1(tn, on), 3),
            onsets=round(f1({t for t, _ in tn}, {t for t, _ in on}), 3),
        ))
    if '--json' in sys.argv:
        json.dump(rows, open(sys.argv[sys.argv.index('--json') + 1], 'w'), indent=1)
    cols = ['id', 'measures', 'meter', 'lengths', 'quarters', 'tempo', 'changes', 'seconds', 'onsets', 'notes']
    print('  '.join(f'{c:>9}' if c != 'id' else f'{c:36}' for c in cols))
    for r in rows:
        if 'error' in r:
            print(f"{r['id']:36}  ERROR {r['error']}")
            continue
        print('  '.join(f'{str(r[c]):>9}' if c != 'id' else f'{r[c]:36}' for c in cols))


if __name__ == '__main__':
    main()
