#!/usr/bin/env python3
"""Behavioral narrowing probes at Load and Launch.Run; restore candidate bytes."""
import os
import pathlib
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parent.parent
out = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else root / '.temp/execution-mutants')
out.mkdir(parents=True, exist_ok=True)
config = root / 'internal/axconfig/config.go'
launch = root / 'internal/execution/execution.go'
reader = root / 'internal/fragment/json.go'
process = root / 'internal/execution/process.go'
shared = root / 'internal/configfile/configfile.go'
mutants = [
 ('F1', process, 'cmd.SysProcAttr.Foreground = true', 'cmd.SysProcAttr.Foreground = !(len(cmd.Args) > 1 && cmd.Args[1] == "start")', 'TestRealPTYOwnershipBothModes/tracked=true', 'withhold foreground ownership only for tracked ax; direct ownership remains'),
 ('F2', process, '_ = syscall.Kill(-int(group), s.(syscall.Signal))', 'if s != os.Interrupt { _ = syscall.Kill(-int(group), s.(syscall.Signal)) }', 'TestRealPTYOwnershipBothModes/tracked=false', 'omit parent-only INT relay while retaining TERM/HUP/QUIT and terminal delivery'),
 ('C1', config, 'len(v.Obj) != 2', '(len(v.Obj) != 2 && !(len(v.Obj) == 3 && v.Obj[2].Key == "locked"))', 'TestLoadClosedSchema/unknown', 'admit exactly one extra trailing locked member'),
 ('C2', config, 'schema.Str != "curator-run-ax-v1"', '(schema.Str != "curator-run-ax-v1" && schema.Str != "other")', 'TestLoadClosedSchema/bad_schema', 'admit exactly schema other'),
 ('C3', config, 'enabled.Kind != fragment.KindBool', 'enabled.Kind != fragment.KindBool && !(enabled.Kind == fragment.KindNull && v.Obj[1].Key == "enabled")', 'TestLoadClosedSchema/null', 'admit null enabled while refusing string/number'),
 ('C4', reader, 'if seen[key] {', 'if seen[key] && key != "enabled" {', 'TestLoadClosedSchema/duplicate', 'admit duplicate enabled only with last-wins object storage'),
 ('C5', config, 'return enabled, nil', 'if !enabled { continue }; return enabled, nil', 'TestLoadPrecedenceBeforeParse', 'ignore only explicit false machine policy'),
 ('C6', shared, 'if !info.Mode().IsRegular() {\n\t\treturn nil, true, fmt.Errorf("configuration file is not a regular file")\n\t}\n\n\tf, err := openFile(path)', 'if info.Mode()&os.ModeNamedPipe!=0{return nil,false,nil};if !info.Mode().IsRegular() {\n\t\treturn nil, true, fmt.Errorf("configuration file is not a regular file")\n\t}\n\n\tf, err := openFile(path)', 'TestLoadFilesystemFailures/fifo', 'treat FIFO only as optional absence'),
 ('C7', shared, 'if info.Mode()&os.ModeSymlink != 0 {\n\t\treturn nil, true, errSymlink', 'if info.Mode()&os.ModeSymlink != 0 {\n if _,err:=os.Stat(path);os.IsNotExist(err){return nil,false,nil};return nil,true,errSymlink', 'TestLoadFilesystemFailures/dangling_file', 'treat dangling final symlinks only as absence'),
 ('C8', shared, 'f, err := openFile(path)\n\tif err != nil {', 'f,err:=openFile(path);if os.IsPermission(err){return nil,false,nil};if err!=nil{', 'TestLoadFilesystemFailures/unreadable_file', 'turn permission-denied open only into fallback'),
 ('C9', shared, 'return nil, false, fmt.Errorf("configuration ancestor %q is not a directory", path)', 'return nil,false,nil', 'TestLoadFilesystemFailures/file_ancestor', 'non-directory ancestor only becomes absence'),
 ('E1', launch, 'if opts.Boundary == nil {', 'if opts.Boundary == nil && !l.tracked { opts.Boundary = func() error { return nil } }; if opts.Boundary == nil {', 'TestLateChecksBothModes/tracked=false/nil_boundary', 'admit absent third boundary in direct mode only'),
 ('E2', launch, 'if err := opts.Boundary(); err != nil {', 'if err := opts.Boundary(); err != nil && !(l.tracked && strings.HasPrefix(err.Error(), "prompt_file_detected:")) {', 'TestLateChecksBothModes/tracked=true/boundary_refusal', 'admit prompt-file refusal in tracked mode only'),
 ('E3', launch, 'if err != nil {\n\t\treturn fail(fmt.Errorf("exec_provider_missing:', 'if err != nil && !(l.tracked && os.IsNotExist(err) && strings.HasSuffix(l.value.Binary, "/provider")) {\n\t\treturn fail(fmt.Errorf("exec_provider_missing:', 'TestLateChecksBothModes/tracked=true/provider_removed', 'admit only absent binary named provider in tracked mode; nonexecutable still refuses'),
 ('E4', launch, 'if err := l.value.CheckLaunchBoundary(); err != nil {', 'if err := l.value.CheckLaunchBoundary(); err != nil && !(l.tracked && strings.HasPrefix(err.Error(), "mcp_layer_missing:")) {', 'TestLateChecksBothModes/tracked=true/mcp_removed', 'admit missing MCP only in tracked mode'),
 ('E5', launch, 'if err := l.value.CheckLaunchBoundary(); err != nil {', 'if err := l.value.CheckLaunchBoundary(); err != nil && !(!l.tracked && strings.HasPrefix(err.Error(), "mcp_layer_missing:")) {', 'TestLateChecksBothModes/tracked=false/mcp_removed', 'admit missing MCP only in direct mode'),
 ('E6', launch, 'fail(errors.New("ax_handoff_failed: ax could not take the launch"))', 'if child, ok := err.(*processExit); ok && child.ExitCode() == 16 { l.tracked = false; return l.Run(opts) }; fail(errors.New("ax_handoff_failed: ax could not take the launch"))', 'TestAxFailureNoFallback/nonzero', 'fallback only for ax exit 16; other ax failures still refuse'),
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
  if ident == 'C4':
   # Merely suppressing the duplicate error leaves three members, independently
   # rejected by Load's cardinality gate. Model actual last-wins admission.
   anchor = 'v.Obj = append(v.Obj, Member{Key: key, Value: val})'
   replacement = 'replaced := false; if key == "enabled" { for i := range v.Obj { if v.Obj[i].Key == key { v.Obj[i].Value = val; replaced = true } } }; if !replaced { ' + anchor + ' }'
   if mutated.count(anchor) != 1:
    raise RuntimeError('C4: object-storage anchor drift')
   mutated = mutated.replace(anchor, replacement)
  path.write_text(mutated)
  # Each probe runs the bounded scoped behavioral suite and must fail its
  # named assertion, including FIFO-as-absence without a hanging read.
  command = ['go', 'test', './internal/axconfig', './internal/execution', '-count=1', '-v', '-timeout=30s']
  if ident in ('F1', 'F2'):
   command = ['go', 'test', './internal/execution', '-count=1', '-v', '-run=TestRealPTYOwnershipBothModes', '-timeout=30s']
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
sys.exit(1 if failed else 0)
