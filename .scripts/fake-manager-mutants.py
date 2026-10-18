#!/usr/bin/env python3
"""Bounded fake-manager source controls with exact assertion and byte restoration."""
import argparse
import hashlib
from pathlib import Path
import re
import subprocess
import sys

root = Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('evidence', nargs='?', default='.temp/fake-manager-mutants')
parser.add_argument('--list', action='store_true')
args = parser.parse_args()
source = root / 'internal/sessionmanager/fakemanager/manager_unix.go'
# These weaken ordinary guards without enabling the reference's known-bad flags.
# Only the target behavioral failure counts: no compilation, setup or timeout
# failure is accepted as a kill. All probes have unmutated positive controls.
mutants = [
 ('F01', 'operation-conflict', [('l.Digest != digest && !m.bad("operation-conflict")', 'l.Digest != digest && false && !m.bad("operation-conflict")')]),
 ('F02', 'operation-expiry', [('m.now.Sub(submitted) > Retention &&', 'm.now.Sub(submitted) > Retention && false &&')]),
 ('F03', 'idempotency', [('if !m.bad("idempotency") &&', 'if false && !m.bad("idempotency") &&')]),
 ('F04', 'cancel-fence', [('l.Outcome == "cancelled" && !m.bad("cancel-fence")', 'l.Outcome == "cancelled" && false && !m.bad("cancel-fence")'), ('!(l.Outcome == "cancelled" && m.bad("cancel-fence"))', 'l.Outcome != "cancelled"')]),
 ('F05', 'reserved-crash', [('if err := m.save(); err != nil {\n\t\treturn refusal', 'if err := error(nil); err != nil {\n\t\treturn refusal')]),
 ('F06', 'admission-launch-launch', [('key != "" && owner != "" && !m.bad("admission")', 'key != "" && owner != "" && false && !m.bad("admission")')]),
 ('F07', 'generation', [('r.ExpectedGeneration && !m.bad("generation")', 'r.ExpectedGeneration && false && !m.bad("generation")')]),
 ('F08', 'not-busy-busy', [('&& !m.bad("not-busy") {', '&& false && !m.bad("not-busy") {')]),
 ('F09', 'pagination', [('end < len(snap.Rows) && !m.bad("pagination")', 'end < len(snap.Rows) && false && !m.bad("pagination")')]),
 ('F10', 'unreadable-index', [('m.control.IndexUnreadable && !m.bad("index-unreadable")', 'm.control.IndexUnreadable && false && !m.bad("index-unreadable")')]),
 ('F11', 'label-subset', [('if subset(s.Labels, r.Labels) ||', 'if true || subset(s.Labels, r.Labels) ||')]),
 ('F12', 'unreadable-row', [('if m.bad("unreadable") {', 'if true || m.bad("unreadable") {')]),
 ('F13', 'started-unreadable', [('if s == nil || m.unreadable(s) {', 'if s == nil {')]),
 ('F14', 'doorbell-uncertain', [('d["state"] = "off"', 'd["state"] = "off"\n\td["uncertain"],d["reason"]=false,nil')]),
 ('F15', 'graceful-retirement', [('m.control.GracefulHang && s.Alive &&', 'm.control.GracefulHang && s.Alive && false &&')]),
 ('F16', 'identity-drift', [('return !ok || p != s.Profile', 'return false && (!ok || p != s.Profile)')]),
 ('F17', 'graceful-retirement', [('s.Alive, s.Gate, s.Retirement = false, false, 4', 's.Alive, s.Gate, s.Retirement = true, false, 4')]),
 ('F18', 'canonical-replay', [('sum := sha256.Sum256(fragment.Canonical(v))', 'sum := sha256.Sum256(data);_ = v')]),
 ('F19', 'admission-recycle', [('if (owner != nil && owner.Handle == s.Handle) ||', 'if true || (owner != nil && owner.Handle == s.Handle) ||')]),
 ('F20', 'profile-digest', [('if !m.bad("profile-digest") {', 'if false && !m.bad("profile-digest") {')]),
 ('F21', 'filtered-index', [('slices.Contains(m.control.MissingIndexLabels, h) &&', 'slices.Contains(m.control.MissingIndexLabels, h) && false &&')]),
 ('F22', 'resume-location', [('&& !m.bad("resume-location") {', '&& false && !m.bad("resume-location") {')]),
 ('F23', 'resume-ambiguous', [('&& !m.bad("resume-ambiguous") {', '&& false && !m.bad("resume-ambiguous") {')]),
 ('F24', 'index-labels', [('encode(projection(s))) != nil', 'encode(projection(s))) != nil && false')]),
 ('F25', 'record-binding', [('return !record.Readable || record.Session != s.Handle || record.Manager != s.Manager || record.Runtime != s.Runtime || !reflect.DeepEqual(record.Labels, s.Labels)', 'return !record.Readable')]),
]
if args.list:
    for ident, prop, _ in mutants:
        print(ident, prop)
    sys.exit(0)
out = (root / args.evidence).resolve()
out.mkdir(parents=True, exist_ok=True)
original = source.read_bytes()
(out / 'candidate.sha256').write_text(hashlib.sha256(original).hexdigest() + '\n')
rows = ['mutant\tproperty\texit\tverdict']
failed = False
for ident, prop, edits in mutants:
    try:
        text = original.decode()
        for old, new in edits:
            if text.count(old) != 1:
                raise RuntimeError(f'{ident}: expected one source anchor, got {text.count(old)}')
            text = text.replace(old, new, 1)
        source.write_text(text)
        if prop in {'canonical-replay', 'index-labels', 'record-binding'}:
            package = './internal/sessionmanager/fakemanager'
            test, marker = {
                'canonical-replay': ('^TestCanonicalReplay$', 'semantic replay changed the result'),
                'index-labels': ('^TestStateBoundaries$/^nil-labels$', 'invalid state accepted'),
                'record-binding': ('^TestUnreadableBindings$/^handle$', 'corrupt record was not projected'),
            }[prop]
        else:
            package = './internal/sessionmanager/conformance'
            tier = 'B' if prop in {'operation-expiry', 'reserved-crash', 'not-busy-busy', 'unreadable-index', 'unreadable-row', 'started-unreadable', 'doorbell-uncertain', 'graceful-retirement', 'identity-drift', 'profile-digest', 'filtered-index', 'resume-ambiguous'} else 'A'
            test = f'^TestConformance$/^{tier}$/^{re.escape(prop)}$'
            marker = 'VIOLATION ' + prop + ':'
        cmd = ['go', 'test', package, '-count=1', '-v', '-timeout=45s', '-run', test]
        with (out / f'{ident}.log').open('w') as log:
            log.write('COMMAND: ' + ' '.join(cmd) + '\n'); log.flush()
            result = subprocess.run(cmd, cwd=root, stdout=log, stderr=subprocess.STDOUT, timeout=60)
            log.write(f'\nEXIT: {result.returncode}\n')
        evidence = (out / f'{ident}.log').read_text(errors='replace')
        killed = result.returncode == 1 and marker in evidence and '--- FAIL:' in evidence and 'SETUP:' not in evidence
        rows.append(f'{ident}\t{prop}\t{result.returncode}\t' + ('killed' if killed else 'SURVIVOR/unverified'))
        failed |= not killed
        print(rows[-1], flush=True)
    finally:
        source.write_bytes(original)
        if source.read_bytes() != original:
            raise RuntimeError(f'{ident}: restore mismatch')
(out / 'summary.tsv').write_text('\n'.join(rows) + '\n')
sys.exit(1 if failed else 0)
