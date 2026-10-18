#!/usr/bin/env python3
"""Bounded Werkstatt controls; restore exact source bytes after each plant."""
import argparse
import pathlib
import re
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parents[1]
p = argparse.ArgumentParser(description=__doc__)
p.add_argument('evidence', nargs='?', default='.temp/werkstatt-mutants')
p.add_argument('--list', action='store_true')
p.add_argument('--only', help='comma-separated control ids')
a = p.parse_args()
service = 'internal/werkstatt/service_unix.go'
store = 'internal/sessionmanager/client/store_unix.go'
# The assertion must name the intended violated property. Compilation, setup,
# timeouts and process-survivor failures do not count as detected controls.
controls = [
 ('W01', 'internal/werkstatt/workspace.go', 'if err := ancestors(physical); err != nil {', 'if err := error(nil); err != nil {', './cmd/curator-run', 'TestWerkstattBinary/admission/physical-leak', 'up exit=0 want=2'),
 ('W02', service, 'if binding(l.Manager.ID, e) != l.Manager {', 'if false {', './cmd/curator-run', 'TestWerkstattBinary/owner-rebind', 'stderr=curator-run: werkstatt_already_up'),
 ('W03', service, 'if l != nil && l.State != "down" {', 'if l != nil && l.State != "down" && l.Manager.ID == selected.ID {', './cmd/curator-run', 'TestWerkstattBinary/manager-switch', 'cross-manager duplicate'),
 ('W04', service, '"expected_generation": v.Generation, "mode": mode', '"expected_generation": l.Generation, "mode": mode', './cmd/curator-run', 'TestWerkstattBinary/current-stop-generation', 'session_generation_mismatch'),
 ('W05', 'internal/sessionmanager/client/capabilities.go', 'if !n.DelegatedTrial {', 'if !n.DelegatedTrial && false {', './cmd/curator-run', 'TestWerkstattBinary/admission/trial-off', 'up exit=0 want=16'),
 ('W06', store, ' || !sameBinding(actual.Manager, expected.Manager)', '', './internal/sessionmanager/client', 'TestLivenessBindingCAS', 'stale binding wrote true'),
 ('W07', store, 'if old.State != "down" || sameOperation', 'if false || sameOperation', './internal/sessionmanager/client', 'TestLivenessBindingCAS', 'live owner replaced true'),
 ('W08', store, 'if !sessionRead && next.OperationID != nil', 'if false && next.OperationID != nil', './internal/sessionmanager/client', 'TestPrestartAbandon', 'late outcome wrote true'),
 ('W09', 'internal/sessionmanager/client/session_json.go', 'if err != nil || s.Readable {', 'if true {', './cmd/curator-run', 'TestWerkstattBinary/unreadable-status', 'invalid status:'),
 ('W10', 'cmd/curator-run/werkstatt_unix.go', 'if err != nil {\n\t\treturn werkstatt.Prepared{}, err\n\t}\n\tif admitted.Plan.Stdin.Attached', 'if err != nil && false {\n\t\treturn werkstatt.Prepared{}, err\n\t}\n\tif admitted.Plan.Stdin.Attached', './cmd/curator-run', 'TestWerkstattBinary/admission/model', 'up exit=0 want=1'),
 ('W11', 'schemas/session-manager/launch.schema.json', '^[A-Za-z0-9._:/@+=-]{1,128}$', '^[A-Za-z0-9._:/@+-]{1,128}$', './cmd/curator-run', 'TestWerkstattBinary/admission/identity-equals', 'up exit=2 want=0'),
]
if a.only:
    wanted = set(a.only.split(','))
    if wanted - {c[0] for c in controls}: p.error('unknown control id')
    controls = [c for c in controls if c[0] in wanted]
if a.list:
    for ident, path, _, _, _, test, _ in controls:
        print(ident, path, test)
    sys.exit(0)
out = (root / a.evidence).resolve()
out.mkdir(parents=True, exist_ok=True)
rows = ['control\ttest\texit\tverdict']
failed = False
for ident, relative, old, new, package, test, assertion in controls:
    path = root / relative
    original = path.read_bytes()
    try:
        source = original.decode()
        if source.count(old) != 1:
            raise RuntimeError(f'{ident}: anchor count {source.count(old)}')
        path.write_text(source.replace(old, new, 1))
        pattern = '/'.join('^' + re.escape(part) + '$' for part in test.split('/'))
        command = ['go', 'test', package, '-count=1', '-v', '-timeout=90s', '-run', pattern]
        with (out / f'{ident}.log').open('w') as log:
            log.write('COMMAND: ' + ' '.join(command) + '\n'); log.flush()
            result = subprocess.run(command, cwd=root, stdout=log, stderr=subprocess.STDOUT, timeout=110)
            log.write(f'\nEXIT: {result.returncode}\n')
        evidence = (out / f'{ident}.log').read_text(errors='replace')
        killed = (result.returncode == 1 and f'--- FAIL: {test} ' in evidence and assertion in evidence
                  and not any(s in evidence for s in ['[build failed]', 'test timed out', 'test command deadline', 'process survived', 'manager survived']))
        rows.append(f'{ident}\t{test}\t{result.returncode}\t' + ('killed' if killed else 'SURVIVOR/unverified'))
        failed |= not killed
        print(rows[-1], flush=True)
    finally:
        path.write_bytes(original)
        if path.read_bytes() != original:
            raise RuntimeError(f'{ident}: restore mismatch')
(out / 'summary.tsv').write_text('\n'.join(rows) + '\n')
sys.exit(1 if failed else 0)
