#!/usr/bin/env python3
"""SPEC5 behavioral narrowing probes; restore exact candidate bytes on exit."""
import pathlib
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parents[1]
out = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else root / '.temp/systemprompt-mutants').resolve()
out.mkdir(parents=True, exist_ok=True)
path = root / 'internal/systemprompt/systemprompt.go'
mutants = [
    ('M1', 'if opt == "" {', 'if opt == "" && f.Environment == fragment.EnvClaudeCode { opt = fragment.SemanticsAppend }; if opt == "" {', 'TestSelectRefusalsAndNoOptIn/claude_code/section/', 'apply append without opt-in for Claude only'),
    ('M2', 'if c.Semantics != opt || c.Kind == fragment.KindFile {', 'if c.Semantics != opt && f.Environment != fragment.EnvCodexCLI || c.Kind == fragment.KindFile {', 'TestSelectRefusalsAndNoOptIn/codex_cli/section/append', 'admit Codex append using replace descriptor'),
    ('M3', 'if c.Semantics != opt || c.Kind == fragment.KindFile {', 'if c.Kind == fragment.KindFile && c.Semantics == opt && opt == fragment.SemanticsReplace { return Selection{}, nil }; if c.Semantics != opt || c.Kind == fragment.KindFile {', 'TestSelectRefusalsAndNoOptIn/pi/section/replace', 'admit matching replace file as an opt-in no-op'),
    ('M4', 'return Selection{}, &Refusal{Code: CodeUnavailable', 'if f.SystemPrompt == nil && opt == fragment.SemanticsAppend { return Selection{}, nil }; return Selection{}, &Refusal{Code: CodeUnavailable', 'TestSelectRefusalsAndNoOptIn/pi/absent/append', 'admit append with absent section'),
    ('M5', 'if f.Environment != fragment.EnvPi {', 'if f.Environment != fragment.EnvPi || f.SystemPrompt == nil {', 'TestPrepareLaunchLateFilesAndWarnings/no-section', 'skip registry probe only without section'),
    ('M6', 'if err != nil {\n\t\treturn fail(err)\n\t}\n\tif !info.Mode().IsRegular()', 'if err != nil {\n\t\tif errors.Is(err, os.ErrNotExist) { return false, nil }; return fail(err)\n\t}\n\tif !info.Mode().IsRegular()', 'TestPrepareLaunchFileFailures/APPEND_SYSTEM.md/dangling', 'treat dangling target as absence'),
    ('M7', 'if !info.Mode().IsRegular() {', 'if info.IsDir() { return true, nil }; if !info.Mode().IsRegular() {', 'TestPrepareLaunchFileFailures/APPEND_SYSTEM.md/directory', 'admit directory only at regular-file check'),
    ('M8', 'file, err := os.Open(path)\n\tif err != nil {', 'file, err := os.Open(path)\n\tif errors.Is(err, os.ErrPermission) { return true, nil }; if err != nil {', 'TestPrepareLaunchFileFailures/APPEND_SYSTEM.md/permissions', 'admit permission-denied open only'),
    ('M9', 'if allowAbsent && errors.Is(err, os.ErrNotExist) {', 'if errors.Is(err, os.ErrNotExist) && (allowAbsent || strings.HasSuffix(path, "prompt.md")) {', 'TestPrepareLaunchFileFailures/selected/missing', 'admit missing selected Pi prompt path'),
    ('M10', 'if s.channel.Kind == fragment.KindFlag && s.channel.Semantics == file.Semantics {', 'if s.channel.Kind == fragment.KindFlag {', 'TestPrepareLaunchLateFilesAndWarnings/section', 'claim suppression for opposite-semantics home file'),
    ('M11', 'observed, err := ProbeFiles(f)', 'observed, err := ProbeFiles(f)\n if s.channel.Kind == fragment.KindFlag && err != nil && strings.HasSuffix(err.(*Refusal).Path, "APPEND_SYSTEM.md") { err = nil }', 'TestPrepareLaunchFileFailures/APPEND_SYSTEM.md/directory', 'bypass file failure only for suppressed append file'),
]
original = path.read_bytes()
rows = ['mutant\tnarrows_to\ttest\texit\tverdict']
failed = False
try:
    for ident, old, new, test, bound in mutants:
        source = original.decode()
        count = source.count(old)
        if count != 1 and not (ident in ('M6', 'M7') and count == 2):
            raise RuntimeError(f'{ident}: anchor count {count}')
        path.write_text(source.replace(old, new, 1))
        command = ['go', 'test', './internal/systemprompt', '-count=1', '-v']
        with (out / f'{ident}.log').open('w') as log:
            log.write('COMMAND: ' + ' '.join(command) + '\n')
            log.flush()
            result = subprocess.run(command, cwd=root, stdout=log, stderr=subprocess.STDOUT, timeout=120)
            log.write(f'\nEXIT: {result.returncode}\n')
        text = (out / f'{ident}.log').read_text()
        killed = result.returncode == 1 and ('--- FAIL: ' + test) in text
        rows.append(f'{ident}\t{bound}\t{test}\t{result.returncode}\t' + ('killed' if killed else 'SURVIVOR/unverified'))
        failed |= not killed
        path.write_bytes(original)
finally:
    path.write_bytes(original)
    if path.read_bytes() != original:
        raise RuntimeError('restore mismatch')
(out / 'summary.tsv').write_text('\n'.join(rows) + '\n')
print('\n'.join(rows))
sys.exit(1 if failed else 0)
