#!/usr/bin/env python3
"""Bounded selector controls; a named behavioral failure must kill each plant."""
import pathlib
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parents[1]
out = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else root / '.temp/selector-mutants').resolve()
out.mkdir(parents=True, exist_ok=True)
config = root / 'internal/selector/config.go'
protocol = root / 'internal/selector/protocol.go'
process = root / 'internal/selector/process_unix.go'
selection = root / 'internal/defaults/selection.go'
append = root / 'internal/selector/append_unix.go'
main = root / 'cmd/curator-run/main.go'
selectors = './internal/selector'
defaults = './internal/defaults'
entry = './cmd/curator-run'
mutants = [
 ('S25', selection, 'if admitter, ok := system.(agentic.EffortAdmitter); ok {', 'if admitter, ok := system.(agentic.EffortAdmitter); ok && false {', defaults, 'TestSelectionNativeEffort'),
 ('S01', config, '!allowed[m.Key]', '!allowed[m.Key] && m.Key != "extra"', selectors, 'TestConfig/unknown'),
 ('S02', config, 'if layer == configfile.User {', 'if layer == configfile.User && false {', selectors, 'TestConfig'),
 ('S03', config, 'timeout.Int > 60', 'timeout.Int > 61', selectors, 'TestConfig/timeout-high'),
 ('S04', config, '!filepath.IsAbs(executable.Str)', 'false', selectors, 'TestConfig/relative'),
 ('S05', config, 'len(prefix.Arr) > 4', 'len(prefix.Arr) > 5', selectors, 'TestConfig/prefix-long'),
 ('S06', protocol, 'if !found {', 'if !found && false {', selectors, 'TestResponse/outside'),
 ('S07', config, '!allowed[m.Key]', '!allowed[m.Key] && m.Key != "unknown"', selectors, 'TestResponse/unknown'),
 ('S08', protocol, '^[A-Za-z0-9._:/@+-]{1,128}$', '^[A-Za-z0-9._:-]{1,128}$', selectors, 'TestResponse'),
 ('S09', protocol, '^gguf-sha256:[0-9a-f]{64}$', '^gguf-sha256:[0-9A-Za-z]{3,64}$', selectors, 'TestResponse/bad-weight'),
 ('S10', protocol, '^[a-z0-9_]{1,64}$', '^[a-z0-9_]{0,64}$', selectors, 'TestResponse/empty-code'),
 ('S11', config, '!allowed[m.Key]', '!allowed[m.Key] && m.Key != "extra"', selectors, 'TestResponse/unknown-refusal'),
 ('S12', protocol, 'if len(data) > Limit {\n\t\treturn bad()', 'if len(data) > Limit && false {\n\t\treturn bad()', selectors, 'TestEnforceNullAndGrammar'),
 ('S13', selection, 'return c.Effort == nil && other.Effort == nil', 'return c.Effort == nil || other.Effort == nil', selectors, 'TestEnforceNullAndGrammar/non-null'),
 ('S14', process, 'if output.overflow {\n\t\treturn Decision{}, unavailable("invalid")', 'if output.overflow && false {\n\t\treturn Decision{}, unavailable("invalid")', selectors, 'TestEnforce/oversize'),
 ('S15', process, 'if ctx.Err() != nil {\n\t\treturn Decision{}, unavailable("timeout")', 'if ctx.Err() != nil {\n\t\treturn Decision{}, unavailable("exit")', selectors, 'TestEnforce/hang'),
 ('S16', process, 'Setsid: true', 'Setsid: false', selectors, 'TestShadow/valid'),
 ('S17', process, 'return &Child{PID: cmd.Process.Pid, Done: done}, nil', '<-done; return &Child{PID: cmd.Process.Pid, Done: done}, nil', selectors, 'TestShadow/hang'),
 ('S18', process, 'if Append(job.Observations, record) != nil {', 'if false {', selectors, 'TestShadow/valid'),
 ('S19', append, 'os.O_APPEND', 'os.O_TRUNC', selectors, 'TestObservations'),
 ('S20', selection, '!flags.Model.Present && selected.Model.Present', 'selected.Model.Present', defaults, 'TestSelectionPrecedence/model-flag'),
 ('S21', selection, '!flags.Effort.Present && selected.Effort.Present', 'selected.Effort.Present', defaults, 'TestSelectionPrecedence/effort-flag'),
 ('S22', main, 'if deps.selectorConfig.Mode == "enforce" {\n\t\t\tdecision, selectionError := selector.Enforce', 'if deps.selectorConfig.Mode == "enforce" || deps.selectorConfig.Mode == "shadow" {\n\t\t\tdecision, selectionError := selector.Enforce', entry, 'TestProductionPipelineGoldens/codex_cli/tracked=false/shadow'),
 ('S23', main, 'resolved.SelectorRuntime = decision.Runtime', 'resolved.SelectorRuntime = decision.Runtime; resolved.Runtime = decision.Runtime', entry, 'TestRunSelectorRuntimeRecordedOnly'),
 ('S24', protocol, '!validRequestUTF8(r)', 'false', selectors, 'TestRequestEnrichment'),
]
rows = ['mutant\ttest\texit\tverdict']
failed = False
for ident, path, old, new, pkg, test in mutants:
    original = path.read_bytes()
    try:
        source = original.decode()
        if source.count(old) != 1:
            raise RuntimeError(f'{ident}: anchor count {source.count(old)} != 1')
        path.write_text(source.replace(old, new, 1))
        command = ['go', 'test', pkg, '-count=1', '-v', '-run', '^' + test.replace('/', '$/^') + '$']
        with (out / f'{ident}.log').open('w') as log:
            log.write('COMMAND: ' + ' '.join(command) + '\n'); log.flush()
            result = subprocess.run(command, cwd=root, stdout=log, stderr=subprocess.STDOUT, timeout=90)
            log.write(f'\nEXIT: {result.returncode}\n')
        evidence = (out / f'{ident}.log').read_text(errors='replace')
        killed = result.returncode == 1 and f'--- FAIL: {test} ' in evidence
        rows.append(f'{ident}\t{test}\t{result.returncode}\t' + ('killed' if killed else 'SURVIVOR/unverified'))
        failed |= not killed
        print(rows[-1], flush=True)
    finally:
        path.write_bytes(original)
        if path.read_bytes() != original:
            raise RuntimeError(f'{ident}: source restore mismatch')
(out / 'summary.tsv').write_text('\n'.join(rows) + '\n')
sys.exit(1 if failed else 0)
