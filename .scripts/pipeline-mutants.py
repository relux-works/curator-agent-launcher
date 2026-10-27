#!/usr/bin/env python3
"""Narrow native production gates in an isolated copy; require named failures."""
import pathlib
import subprocess
import sys
import tempfile

source = pathlib.Path(__file__).resolve().parents[1]
root = source
out = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else root / '.temp/pipeline-mutants').resolve()
out.mkdir(parents=True, exist_ok=True)
scratch = tempfile.TemporaryDirectory(prefix='candidate-', dir=out)
root = pathlib.Path(scratch.name) / 'tree'
root.mkdir()
for entry in source.iterdir():
 if entry.name not in ('.git', '.temp'):
  subprocess.run(['cp', '-R', str(entry), str(root)], check=True)

execution = root / 'internal/execution/execution.go'
main = root / 'cmd/curator-run/main.go'
mutants = [
 ('E1', execution, 'if err := opts.Boundary(); err != nil {', 'if err := opts.Boundary(); err != nil && false {', 'TestProductionLateChecks/prompt-directory/native', 'omit prompt refusal'),
 ('E2', execution, 'if err != nil {\n\t\treturn fail(fmt.Errorf("exec_provider_missing:', 'if err != nil && false {\n\t\treturn fail(fmt.Errorf("exec_provider_missing:', 'TestProductionLateChecks/binary-not-executable/native', 'omit binary refusal'),
 ('E3', execution, 'if err := l.value.CheckLaunchBoundary(); err != nil {', 'if err := l.value.CheckLaunchBoundary(); err != nil && false {', 'TestProductionLateChecks/mcp-directory/native', 'omit MCP refusal'),
 ('E4', root/'internal/composition/probe.go', 'if !info.Mode().IsRegular() {', 'if !info.Mode().IsRegular() && !info.IsDir() {', 'TestProductionLateChecks/mcp-directory/native', 'admit directories while refusing other nonregular MCP objects'),
 ('E5', root/'internal/systemprompt/systemprompt.go', 'if !info.Mode().IsRegular() {', 'if !info.Mode().IsRegular() && !info.IsDir() {', 'TestProductionLateChecks/discovery-directory/native', 'admit directories while refusing other nonregular prompt objects'),
 ('E6', execution, '!info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0', '!info.Mode().IsRegular()', 'TestProductionLateChecks/binary-not-executable/native', 'admit regular nonexecutable provider files'),
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
scratch.cleanup()
sys.exit(1 if failed else 0)
