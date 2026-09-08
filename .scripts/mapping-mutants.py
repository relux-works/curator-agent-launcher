#!/usr/bin/env python3
"""Task A1 narrowing probes; restore exact candidate bytes, never from Git."""
import pathlib
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parents[1]
out = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else root / '.temp/mapping-mutants').resolve()
out.mkdir(parents=True, exist_ok=True)
mapping = root / 'internal/mapping/mapping.go'
main = root / 'cmd/curator-run/main.go'
mutants = [
    ('M1', mapping, '\tdefault:', '\tcase fragment.EnvOpenCode:\n\t\treturn Target{System: "opencode", Provider: "opencode"}, nil\n\tdefault:', 'TestRunMapping/opencode', 'admit exactly opencode'),
    ('M2', mapping, '\tdefault:', '\tcase "future_env":\n\t\treturn Target{System: "future_env", Provider: "future_env"}, nil\n\tdefault:', 'TestRunUnknownResolvedMapping', 'admit exactly future_env'),
    ('M3', main, 'if err != nil {\n\t\tfmt.Fprintf(stderr, "%s: %s: %v\\n", name, mapping.CodeUnsupported, err)', 'if err != nil && frag.Environment != "opencode" {\n\t\tfmt.Fprintf(stderr, "%s: %s: %v\\n", name, mapping.CodeUnsupported, err)', 'TestRunMapping/opencode', 'continue after opencode mapping error only'),
    ('M4', mapping, 'System: "pi-native"', 'System: "pi"', 'TestRunMapping/pi', 'route Pi to legacy wrapper system'),
]
rows = ['mutant\tnarrows_to\ttest\texit\tverdict']
failed = False
for ident, path, old, new, test, bound in mutants:
    original = path.read_bytes()
    try:
        source = original.decode()
        if source.count(old) != 1:
            raise RuntimeError(f'{ident}: anchor is not unique')
        path.write_text(source.replace(old, new, 1))
        command = ['go', 'test', './cmd/curator-run', './internal/mapping', '-count=1', '-v']
        with (out / f'{ident}.log').open('w') as log:
            log.write('COMMAND: ' + ' '.join(command) + '\n')
            log.flush()
            result = subprocess.run(command, cwd=root, stdout=log, stderr=subprocess.STDOUT)
            log.write(f'\nEXIT: {result.returncode}\n')
        killed = result.returncode == 1 and f'--- FAIL: {test} ' in (out / f'{ident}.log').read_text()
        rows.append(f'{ident}\t{bound}\t{test}\t{result.returncode}\t' + ('killed' if killed else 'SURVIVOR/unverified'))
        failed |= not killed
    finally:
        path.write_bytes(original)
        if path.read_bytes() != original:
            raise RuntimeError(f'{ident}: restore mismatch')
(out / 'summary.tsv').write_text('\n'.join(rows) + '\n')
print('\n'.join(rows))
sys.exit(1 if failed else 0)
