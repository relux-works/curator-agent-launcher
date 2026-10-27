#!/usr/bin/env python3
"""Bounded host-resolution and delegated-launch controls in an isolated copy."""
import argparse
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile

root = Path(__file__).resolve().parents[1]
p = argparse.ArgumentParser(description=__doc__)
p.add_argument('evidence', nargs='?', default='.temp/host-launch-mutants')
p.add_argument('--list', action='store_true')
args = p.parse_args()
main = 'cmd/curator-run/main.go'
host = 'cmd/curator-run/host_selection.go'
delegated = 'cmd/curator-run/delegated.go'
caps = 'internal/sessionmanager/client/capabilities.go'
# id, source, exact plant, replacement, package, exact behavioral failure, evidence
controls = [
 ('M-H1', 'internal/defaults/defaults.go', 'if member.Key == "host" && schema == SchemaV3 {', 'if member.Key == "host" && schema == SchemaV3 && member.Value.Str != HostNative {', './internal/defaults', 'TestHostMemberWithdrawn/machine/native', 'host_member_withdrawn'),
 ('M-H2', main, 'selected, err := resolveHost(ctx, inv, frag, stderr, deps)\n\tif err != nil {\n\t\treturn emitHostFailure(stderr, err)\n\t}', 'selected, err := resolveHost(ctx, inv, frag, stderr, deps)\n\tif err != nil { selected.route.Kind = "native" }', './cmd/curator-run', 'TestFrontDoorBinary/no-fallback/missing-capability', 'want=6'),
 ('M-H3', host, 'if cli.ExplicitNative(inv.HostFlag) || (inv.HostFlag == "" && mode == "native") {', 'if (cli.ExplicitNative(inv.HostFlag) && false) || (inv.HostFlag == "" && mode == "native") {', './cmd/curator-run', 'TestFrontDoorBinary/registry-paths/invalid/--native', 'want=0'),
 ('M-H22', host, 'state, err := sessionmanager.Resolve(registry, sessionmanager.ExecutableAvailable)\n\tif err != nil {\n\t\treturn result, err\n\t}', 'state, err := sessionmanager.Resolve(registry, sessionmanager.ExecutableAvailable)\n\tif err != nil { state.Kind = "empty"; err = nil }', './cmd/curator-run', 'TestFrontDoorBinary/no-fallback/missing-executable', 'want=1'),
 ('S4-01', host, 'if explicit {\n\t\tif err = checks();', 'if false {\n\t\tif err = checks();', './cmd/curator-run', 'TestFrontDoorBinary/refusal-order/--hosted/network/invalid', 'want=16'),
 ('S4-02', host, 'if explicit {\n\t\tif err = checks();', 'if true {\n\t\tif err = checks();', './cmd/curator-run', 'TestFrontDoorBinary/refusal-order//network/invalid', 'want=2'),
 ('S4-04', host, 'if inv.NetworkSet {', 'if inv.NetworkSet && false {', './cmd/curator-run', 'TestFrontDoorBinary/refusal-order/--hosted/network/invalid', 'want=16'),
 ('S4-05', host, 'if !terminal() {', 'if !terminal() && false {', './cmd/curator-run', 'TestFrontDoorBinary/refusal-order/--hosted/stdin/invalid', 'want=6'),
 ('S4-06', host, '(inv.HostFlag == "" && mode == "native")', '(inv.HostFlag == "" && mode == "native" && false)', './cmd/curator-run', 'TestFrontDoorBinary/activation/native-mode-no-trial', 'calls='),
 ('S4-07', caps, 'if !n.DelegatedTrial {', 'if !n.DelegatedTrial && false {', './internal/sessionmanager/client', 'TestActivationMatrix/explicit0', 'wanted session_host_delegated_unauthorized'),
 ('S4-08', caps, 'case n.HostMode == "hosted":', 'case n.HostMode == "hosted" && false:', './internal/sessionmanager/client', 'TestActivationMatrix/hosted-default0', 'wanted session_host_missing'),
 ('S4-09', caps, 'if route == "delegated" && !n.SignedOff {', 'if route == "delegated" && !n.SignedOff && n.HostMode != "auto" {', './internal/sessionmanager/client', 'TestActivationMatrix/auto1', 'activation native'),
 ('S4-10', host, 'if result.route.Kind == "native" {\n\t\thostNotice(stderr, len(registry.Managers))', 'if result.route.Kind == "native" {\n\t\thostNotice(stderr, 0)', './cmd/curator-run', 'TestFrontDoorBinary/activation/auto-trial', 'no session manager selected (1 registered)'),
 ('S4-11', delegated, 'labels["selector"] = resolved.SelectorDecisionID', 'labels["selector"] = "changed"', './cmd/curator-run', 'TestDelegatedProjection', 'projection labels='),
 ('S4-12', delegated, 'goal = "workspace"', 'goal = "none"', './cmd/curator-run', 'TestDelegatedProjection', 'projection board_goal='),
 ('S4-13', delegated, 'session, err := deps.hostedSelection.client.Start(ctx, op, data)', 'session, err := deps.hostedSelection.client.Start(ctx, op, data)\n if err == nil { return 0 }', './cmd/curator-run', 'TestFrontDoorBinary/delegated-details/request', 'calls='),
 ('S4-14', delegated, 'return result.Exit', 'return 0', './cmd/curator-run', 'TestFrontDoorBinary/delegated-details/attach-exit', 'want=7'),
 ('S4-15', delegated, '"expected_generation": session.Generation', '"expected_generation": session.Generation + 1', './cmd/curator-run', 'TestFrontDoorBinary/delegated-details/request', 'want=0'),
 ('S4-16', delegated, '"fragment_digest": frag.Digest', '"fragment_digest": frag.Profile.LockSHA256', './cmd/curator-run', 'TestDelegatedProjection', 'launch_plan_invalid'),
 ('S4-17', delegated, '"provider_id", "id": intent.Identity', '"provider_id", "id": "changed"', './cmd/curator-run', 'TestFrontDoorBinary/delegated-details/resume-provider-id', 'want=0'),
 ('S4-18', 'schemas/session-manager/launch.schema.json', '"additionalProperties": {\n        "type": "string",\n        "pattern": "^[A-Za-z0-9._:/@+-]{1,128}$"\n      }', '"additionalProperties": {\n        "type": "string",\n        "pattern": "^[A-Za-z0-9._@:-]{1,128}$"\n      }', './cmd/curator-run', 'TestDelegatedProjection', 'launch_plan_invalid'),
 ('S4-19', delegated, '"cwd": cwd, "labels": map[string]string{}', '"cwd": cwd, "labels": labels', './cmd/curator-run', 'TestDelegatedProjection', 'resume projection:'),
]
if args.list:
    print('\n'.join(c[0] for c in controls))
    raise SystemExit(0)
selected = set(filter(None, os.environ.get('MUTANTS_ONLY', '').split(',')))
if selected - {c[0] for c in controls}:
    p.error('unknown mutation id')
controls = [c for c in controls if not selected or c[0] in selected]
out = Path(args.evidence).resolve()
out.mkdir(parents=True, exist_ok=True)
summary = ['mutant\tfile\texpected_failing_test\tapplied\tsuite_exit\texpected_test_failed\tverdict']
failed = False

def run_test(work, package, test, log):
    pattern = '/'.join('^' + re.escape(part) + '$' for part in test.split('/'))
    command = ['go', 'test', package, '-run', pattern, '-count=1', '-v', '-timeout=180s']
    try:
        result = subprocess.run(command, cwd=work, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=210)
        text = result.stdout.decode(errors='replace')
        log.write_text(text)
        return result.returncode, text
    except subprocess.TimeoutExpired as e:
        log.write_bytes(e.stdout or b'')
        return 124, ''

with tempfile.TemporaryDirectory(prefix='candidate-', dir=out) as scratch:
    work = Path(scratch) / 'tree'
    shutil.copytree(root, work, symlinks=True, ignore=shutil.ignore_patterns('.git', '.temp'))
    baselines = {}
    for ident, source, before, after, package, test, evidence in controls:
        path = work / source
        original = path.read_bytes()
        text = original.decode()
        verdict, applied, code, expected_failed = 'NOT_APPLIED', 'no', '-', 'no'
        if text.count(before) != 1:
            print(f'NOT_APPLIED: {ident} matched {text.count(before)} sources in {source}', file=sys.stderr, flush=True)
        else:
            key = (package, test)
            if key not in baselines:
                baseline_code, baseline_text = run_test(work, package, test, out / (ident + '-control.log'))
                baselines[key] = baseline_code == 0 and ('--- PASS: ' + test) in baseline_text
            if not baselines[key]:
                verdict = 'BASELINE_FAILED'
            else:
                try:
                    path.write_text(text.replace(before, after, 1))
                    applied = 'yes'
                    code, result = run_test(work, package, test, out / (ident + '.log'))
                    expected_failed = 'yes' if re.search(r'^\s*--- FAIL: ' + re.escape(test) + r'(?: |$)', result, re.M) and evidence in result else 'no'
                    verdict = 'KILLED' if code == 1 and expected_failed == 'yes' else 'SURVIVED'
                finally:
                    path.write_bytes(original)
                if path.read_bytes() != original:
                    raise RuntimeError('candidate source not restored')
        summary.append(f'{ident}\t{source}\t{test}\t{applied}\t{code}\t{expected_failed}\t{verdict}')
        (out / 'summary.tsv').write_text('\n'.join(summary) + '\n')
        print(f'{ident}: {verdict}', flush=True)
        failed |= verdict != 'KILLED'
raise SystemExit(int(failed))
