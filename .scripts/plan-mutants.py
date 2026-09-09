#!/usr/bin/env python3
"""SPEC 4.4 narrowing probes at plan.Build; restore exact uncommitted bytes."""
import pathlib
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parents[1]
out = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else root / '.temp/plan-mutants').resolve()
out.mkdir(parents=True, exist_ok=True)
path = root / 'internal/plan/plan.go'
mutants = [
 ('P1', 'strings.TrimSpace(req.Home) == ""', 'req.Home == ""', 'TestRequiredInputs/home_whitespace', 'admit whitespace-only home'),
 ('P2', 'strings.TrimSpace(req.WorkDir) == ""', 'req.WorkDir == ""', 'TestRequiredInputs/workdir_whitespace', 'admit whitespace-only workdir'),
 ('P3', 'if d.BuildLaunch == nil {', 'if d.BuildLaunch == nil && req.Runtime == "codex" { d.BuildLaunch = vendorplugin.BuildLaunch }; if d.BuildLaunch == nil {', 'TestRequiredInputs/build_nil', 'default missing launch dependency for Codex only'),
 ('P4', 'if d.Availability == nil {', 'if d.Availability == nil && req.Runtime == "codex" { d.Availability = func(providerlimits.VerdictQuery) (vendorplugin.Availability, error) { return vendorplugin.Availability{State: vendorplugin.AvailabilityHealthy}, nil } }; if d.Availability == nil {', 'TestRequiredInputs/availability_nil', 'admit missing limits reader for Codex only'),
 ('P5', 'if err != nil {\n\t\treturn agentic.Plan{}, &RefusedError{Detail: "spawn plane refused', 'if err != nil && !errors.Is(err, vendorplugin.ErrEffortMissing) {\n\t\treturn agentic.Plan{}, &RefusedError{Detail: "spawn plane refused', 'TestTaggedAdmissionRefusals/missing_effort', 'admit required-effort refusal only'),
 ('P6', 'if !verdict.Serviceable() {', 'if !verdict.Serviceable() && verdict.State != vendorplugin.AvailabilityUnknown {', 'TestProviderVerdicts/unknown', 'admit unknown state only'),
 ('P7', 'if err != nil {\n\t\t// Failure to produce', 'if err != nil && err.Error() != "module read failure" {\n\t\t// Failure to produce', 'TestProviderReadError', 'admit one read-error value while keeping error gate'),
 ('P8', 'Home:    req.Home,\n\t})', 'Home:    "",\n\t})', 'TestTaggedInteractivePlans/claude_code', 'substitute native home for limits query'),
 ('P9', 'Model:   req.Model,\n\t\tHome:', 'Model:   "claude-fable-5",\n\t\tHome:', 'TestTaggedInteractivePlans/claude_code', 'check a different model group'),
 ('P10', 'Runtime: req.Runtime,\n\t\tModel:', 'Runtime: "codex",\n\t\tModel:', 'TestTaggedInteractivePlans/claude_code', 'check Codex state for Claude request'),
 ('P11', 'spawn := SpawnRequest(req)', 'spawn := SpawnRequest(req)\n if spawn.Effort == "" && spawn.Model == "claude-opus-5" { spawn.Effort = "medium" }', 'TestTaggedAdmissionRefusals/missing_effort', 'default effort only for one required model'),
 ('P12', 'spawn := SpawnRequest(req)', 'spawn := SpawnRequest(req)\n if spawn.Effort == "bogus" { spawn.Effort = "medium" }', 'TestTaggedAdmissionRefusals/invalid_effort', 'downgrade one invalid effort word'),
 ('P13', 'if err != nil {\n\t\treturn agentic.Plan{}, &RefusedError{Detail: "spawn plane refused', 'if err != nil && !errors.Is(err, vendorplugin.ErrModelNotDrivenBySystem) {\n\t\treturn agentic.Plan{}, &RefusedError{Detail: "spawn plane refused', 'TestModelNotDrivenBySystem', 'admit model-not-driven-by-system refusal only'),
]
original = path.read_bytes()
rows = ['mutant\tbound\ttest\texit\tverdict']
failed = False
for ident, old, new, test, bound in mutants:
    try:
        source = original.decode()
        if source.count(old) != 1:
            raise RuntimeError(f'{ident}: anchor is not unique')
        path.write_text(source.replace(old, new, 1))
        command = ['go', 'test', './internal/plan', '-count=1', '-v']
        with (out / f'{ident}.log').open('w') as log:
            log.write('COMMAND: ' + ' '.join(command) + '\n')
            log.flush()
            result = subprocess.run(command, cwd=root, stdout=log, stderr=subprocess.STDOUT, timeout=90)
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
