#!/usr/bin/env python3
"""Narrow production gates; require named run-entry failures and restore bytes."""
import pathlib
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parents[1]
out = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else root / '.temp/pipeline-mutants').resolve()
out.mkdir(parents=True, exist_ok=True)
execution = root / 'internal/execution/execution.go'
main = root / 'cmd/curator-run/main.go'
mutants = [
 ('E1', execution, 'if err := opts.Boundary(); err != nil {', 'if err := opts.Boundary(); err != nil && !l.tracked {', 'TestProductionLateChecks/prompt-directory/tracked=true', 'prompt refusal only in direct mode'),
 ('E2', execution, 'if err != nil {\n\t\treturn fail(fmt.Errorf("exec_provider_missing:', 'if err != nil && !l.tracked {\n\t\treturn fail(fmt.Errorf("exec_provider_missing:', 'TestProductionLateChecks/binary-not-executable/tracked=true', 'binary refusal only in direct mode'),
 ('E3', execution, 'if err := l.value.CheckLaunchBoundary(); err != nil {', 'if err := l.value.CheckLaunchBoundary(); err != nil && !l.tracked {', 'TestProductionLateChecks/mcp-directory/tracked=true', 'MCP refusal only in direct mode'),
 ('E4', root/'internal/composition/probe.go', 'if !info.Mode().IsRegular() {', 'if !info.Mode().IsRegular() && !info.IsDir() {', 'TestProductionLateChecks/mcp-directory/tracked=false', 'admit directories while refusing other nonregular MCP objects'),
 ('E5', root/'internal/systemprompt/systemprompt.go', 'if !info.Mode().IsRegular() {', 'if !info.Mode().IsRegular() && !info.IsDir() {', 'TestProductionLateChecks/discovery-directory/tracked=false', 'admit directories while refusing other nonregular prompt objects'),
 ('E6', execution, '!info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0', '!info.Mode().IsRegular()', 'TestProductionLateChecks/binary-not-executable/tracked=true', 'admit regular nonexecutable provider files'),
 ('M1', main, 'opts := cli.Options{AxConfigured: configured}', 'opts := cli.Options{AxConfigured: configured && len(args) > 0 && args[0] != "pi"}', 'TestProductionModeSelection/operator-enabled', 'ignore enabled ax only for Pi'),
 ('M2', root/'internal/axconfig/config.go', 'return enabled, nil', 'if !enabled { continue }; return enabled, nil', 'TestProductionModeSelection/machine-disabled', 'ignore machine false only'),
 ('M3', main, 'if err != nil {\n\t\treturn emitFailure(stderr, diagnostics.CodeDefaultsInvalid, err)\n\t}\n\topts', 'if err != nil && len(args) == 0 {\n\t\treturn emitFailure(stderr, diagnostics.CodeDefaultsInvalid, err)\n\t}\n\topts', 'TestProductionModeSelection/malformed', 'ignore malformed ax when argv is nonempty'),
]
rows = ['mutant\tbound\ttest\texit\tverdict']
failed = False
for ident, path, old, new, test, bound in mutants:
    original = path.read_bytes()
    try:
        source = original.decode()
        expected = 2 if ident in ('E4', 'E5') else 1
        if source.count(old) != expected:
            raise RuntimeError(f'{ident}: anchor count {source.count(old)} != {expected}')
        path.write_text(source.replace(old, new))
        command = ['go', 'test', './cmd/curator-run', '-count=1', '-v', '-run', '^' + test.replace('/', '$/^') + '$']
        with (out / f'{ident}.log').open('w') as log:
            log.write('COMMAND: ' + ' '.join(command) + '\n'); log.flush()
            result = subprocess.run(command, cwd=root, stdout=log, stderr=subprocess.STDOUT, timeout=90)
            log.write(f'\nEXIT: {result.returncode}\n')
        killed = result.returncode == 1 and f'--- FAIL: {test} ' in (out / f'{ident}.log').read_text(errors='replace')
        rows.append(f'{ident}\t{bound}\t{test}\t{result.returncode}\t' + ('killed' if killed else 'SURVIVOR/unverified'))
        failed |= not killed
        print(rows[-1], flush=True)
    finally:
        path.write_bytes(original)
        if path.read_bytes() != original:
            raise RuntimeError(f'{ident}: restore mismatch')
(out / 'summary.tsv').write_text('\n'.join(rows) + '\n')
sys.exit(1 if failed else 0)
