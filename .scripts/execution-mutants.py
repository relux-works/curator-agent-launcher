#!/usr/bin/env python3
"""Native Launch.Run narrowing probes; mutate only an isolated copy."""
import os
import pathlib
import subprocess
import sys
import tempfile

source = pathlib.Path(__file__).resolve().parent.parent
root = source
out = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else root / '.temp/execution-mutants')
out = out.resolve()
out.mkdir(parents=True, exist_ok=True)
scratch = tempfile.TemporaryDirectory(prefix='candidate-', dir=out)
root = pathlib.Path(scratch.name) / 'tree'
root.mkdir()
for entry in source.iterdir():
 if entry.name not in ('.git', '.temp'):
  subprocess.run(['cp', '-R', str(entry), str(root)], check=True)

launch = root / 'internal/execution/execution.go'
process = root / 'internal/execution/process.go'
mutants = [
 ('F2', process, '_ = syscall.Kill(-int(group), s.(syscall.Signal))', 'if s != os.Interrupt { _ = syscall.Kill(-int(group), s.(syscall.Signal)) }', 'TestRealPTYOwnership/native', 'omit parent-only INT relay while retaining TERM/HUP/QUIT and terminal delivery'),
 ('E1', launch, 'if opts.Boundary == nil {', 'if opts.Boundary == nil { opts.Boundary = func() error { return nil } }; if opts.Boundary == nil {', 'TestLateChecks/nil_boundary', 'admit absent third boundary in direct mode only'),
 ('E5', launch, 'if err := l.value.CheckLaunchBoundary(); err != nil {', 'if err := l.value.CheckLaunchBoundary(); err != nil && !strings.HasPrefix(err.Error(), "mcp_layer_missing:") {', 'TestLateChecks/mcp_removed', 'admit missing MCP only in direct mode'),
 ('E7', launch, 'Env: append([]string{}, l.value.Env...)', 'Env: append([]string(nil), l.value.Env...)', 'TestEmptyEnvironmentAndLaunchPATH/empty', 'inherit parent only for empty fullEnv'),
]
rows = ['mutant\tnarrows_to\tnamed_failing_test\texit\tverdict\tsurvival_bound']
failed = False
selected = set(os.environ.get("EXECUTION_MUTANT_IDS", "").split())
for ident, path, old, new, test, bound in mutants:
 if selected and ident not in selected:
  continue
 original = path.read_bytes()
 try:
  source = original.decode()
  expected = 1
  if source.count(old) != expected:
   raise RuntimeError(f'{ident}: expected {expected} anchors, got {source.count(old)}')
  mutated = source.replace(old, new)
  path.write_text(mutated)
  # Each probe runs the bounded scoped behavioral suite and must fail its
  # named native execution assertion.
  command = ['go', 'test', './internal/execution', '-count=1', '-v', '-timeout=30s']
  if ident == 'F2':
   command = ['go', 'test', './internal/execution', '-count=1', '-v', '-run=TestRealPTYOwnership', '-timeout=30s']
  with (out / f'{ident}.log').open('w') as log:
   log.write('COMMAND: ' + ' '.join(command) + '\n'); log.flush()
   result = subprocess.run(command, cwd=root, stdout=log, stderr=subprocess.STDOUT, timeout=90)
   log.write(f'\nEXIT: {result.returncode}\n')
  evidence = (out / f'{ident}.log').read_text()
  killed = result.returncode == 1 and (f'--- FAIL: {test} ' in evidence)
  rows.append(f'{ident}\t{bound}\t{test if killed else "none"}\t{result.returncode}\t' + ('killed\tnone' if killed else 'SURVIVOR\t' + bound + ' is not proven on this host'))
  failed |= not killed
 finally:
  path.write_bytes(original)
  if path.read_bytes() != original:
   raise RuntimeError(f'{ident}: restoration failed')
(out / 'summary.tsv').write_text('\n'.join(rows) + '\n')
print('\n'.join(rows))
scratch.cleanup()
sys.exit(1 if failed else 0)
