#!/usr/bin/env python3
"""Run scoped behavioral narrowing mutants; restore the uncommitted candidate."""
import pathlib
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parent.parent
out = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else root / '.temp/composition-mutants')
out.mkdir(parents=True, exist_ok=True)
composer = root / 'internal/composition/composition.go'
probe = root / 'internal/composition/probe.go'
mutants = [
 ('M1', composer, 'literals := maps.Clone(own)', 'literals := maps.Clone(own); literals["SECRET"] = env["SECRET"]', 'TestComposeEnvironmentBoundary', 'admit exactly inherited SECRET as a tracked literal'),
 ('M2', composer, 'if _, collision := literals[name]; collision {', 'if _, collision := literals[name]; collision && name != "OWN" {', 'TestComposeEnvironmentBoundary', 'admit OWN as both literal and lookup'),
 ('M3', composer, 'env := envMap(plan.Env)', 'env := envMap(plan.Env); env["REMOVED"] = envMap(req.Env)["REMOVED"]', 'TestComposeEnvironmentBoundary', 're-admit exactly plugin-removed REMOVED'),
 ('M4', composer, 'if err != nil {', 'if err != nil && err.Error() != "ownership failed" {', 'TestComposeOwnershipFailure', 'admit one ChildEnv failure'),
 ('M5', probe, 'return fail(CodeMCPLayerMissing, err)', 'return nil', 'TestLaunchBoundaryFilesystem/missing', 'admit absent layer while retaining all unreadable gates'),
 ('M6', probe, 'info, err := os.Stat(path)', 'info, err := os.Stat(path); if errors.Is(err, os.ErrNotExist) { return nil }', 'TestLaunchBoundaryFilesystem/dangling', 'admit dangling symlink after successful lstat'),
 ('M7', probe, 'if !info.Mode().IsRegular() {', 'if !info.Mode().IsRegular() && !info.IsDir() {', 'TestLaunchBoundaryFilesystem/directory', 'admit directories but retain other nonregular rejection'),
 ('M8', probe, 'f, err := os.Open(path)', 'f, err := os.Open(path); if errors.Is(err, os.ErrPermission) { return nil }', 'TestLaunchBoundaryFilesystem/unreadable', 'admit permission-denied open only'),
 ('M9', composer, 'if plan.Stdin.Attached {', 'if plan.Stdin.Attached && len(plan.Stdin.Bytes) > 0 {', 'TestComposeStdin/empty', 'collapse attached empty only to null'),
]
rows = ['mutant\tnarrows_to\tnamed_failing_test\texit\tverdict\tsurvival_bound']
failed = False
for ident, path, old, new, test, bound in mutants:
 original = path.read_bytes()
 try:
  source = original.decode()
  expected = 2 if ident == 'M7' else 1
  if source.count(old) != expected:
   raise RuntimeError(f'{ident}: expected {expected} anchors, got {source.count(old)}')
  path.write_text(source.replace(old, new))
  command = ['go', 'test', './internal/composition', '-count=1', '-v']
  with (out / f'{ident}.log').open('w') as log:
   log.write('COMMAND: ' + ' '.join(command) + '\n'); log.flush()
   result = subprocess.run(command, cwd=root, stdout=log, stderr=subprocess.STDOUT)
   log.write(f'\nEXIT: {result.returncode}\n')
  killed = result.returncode == 1 and f'--- FAIL: {test} ' in (out / f'{ident}.log').read_text()
  rows.append(f'{ident}\t{bound}\t{test if killed else "none"}\t{result.returncode}\t' + ('killed\tnone' if killed else 'SURVIVOR\t' + bound + ' remains unproven on this host'))
  failed |= not killed
 finally:
  path.write_bytes(original)
  if path.read_bytes() != original:
   raise RuntimeError(f'{ident}: restoration failed')
(out / 'summary.tsv').write_text('\n'.join(rows) + '\n')
print('\n'.join(rows))
sys.exit(1 if failed else 0)
