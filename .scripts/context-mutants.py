#!/usr/bin/env python3
"""Narrow carrier engagement and entry gates; require named behavioral failures."""
import pathlib
import os
import shutil
import subprocess
import sys
import tempfile

root = pathlib.Path(__file__).resolve().parents[1]
out = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else root / '.temp/context-mutants').resolve()
out.mkdir(parents=True, exist_ok=True)
# Mutants execute in an isolated copy, never in the managed candidate. Include
# uncommitted new files, but exclude ignored scratch and machine configuration.
source_root = root
root = pathlib.Path(tempfile.mkdtemp(prefix='candidate-', dir=out))
files = subprocess.check_output(['git', 'ls-files', '--cached', '--others', '--exclude-standard', '-z'], cwd=source_root)
for name in files.decode().split('\0'):
    if not name:
        continue
    source = source_root / name
    if source.is_file():
        target = root / name
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, target)
plan = root / 'internal/plan/plan.go'
cli = root / 'internal/cli/cli.go'
composition = root / 'internal/composition/composition.go'
mutants = []
for ident, runtime, flag, channel, test in [
    ('C1', 'claude', '--mcp-config', 'MCP', 'claude_code/--mcp-config/false'),
    ('C2', 'claude', '--system-prompt-file=other.md', 'SystemPrompt', 'claude_code/--system-prompt-file/false'),
    ('C3', 'codex', '--profile', 'MCP', 'codex_cli/--profile/true'),
    ('C4', 'codex', '-c', 'SystemPrompt', 'codex_cli/-c/true'),
]:
    old = 'func SpawnRequest(req Request) vendorplugin.SpawnRequest {\n'
    new = old + '\tif req.Context != nil && req.Runtime == "' + runtime + '" && len(req.NativeArgs) > 0 && req.NativeArgs[0] == "' + flag + '" {\n\t\tcopy := *req.Context\n\t\tcopy.' + channel + ' = nil\n\t\treq.Context = &copy\n\t}\n'
    mutants.append((ident, plan, old, new, 'TestProductionContextCarrierRefusesNativeConflicts/' + test,
                    'engage the ' + channel + ' conflict gate except for the named native selector'))
for ident, alias in [('A1', 'openai-infra'), ('A2', 'anthropic-infra')]:
    old = 'tok == "openai-infra" || tok == "anthropic-infra"'
    new = 'tok == "' + ('anthropic-infra' if alias == 'openai-infra' else 'openai-infra') + '"'
    mutants.append((ident, cli, old, new, 'TestProductionDeprecatedInfraAliasesRefused/' + alias,
                    'reject only the other deprecated alias; admit ' + alias + ' to resolution'))
mutants.append(('N1', composition, 'if plan.Argv[cut+i] != arg {',
                'if plan.Argv[cut+i] != arg && arg != "" {',
                'TestProductionContextCarrierRefusesAdmittedNativeSuffixDrift/true',
                'validate suffix drift except for the empty native argument'))
projection = root / 'internal/plan/context.go'
main = root / 'cmd/curator-run/main.go'
mutants.extend([
    ('R1', projection, '\tchannels := func(source []fragment.Channel)',
     '\tif f.Environment == fragment.EnvClaudeCode { value.PathPrepend = f.PathPrepend }\n\tchannels := func(source []fragment.Channel)',
     'TestProductionContextCarrierReservedMembersPreservePathAndDigest/claude_code/false',
     'exclude reserved fields except forwarding path_prepend for Claude only'),
    ('P1', projection, 'if f.SystemPrompt != nil && intent != "" {',
     'if f.SystemPrompt != nil && intent != "" && !(f.Environment == fragment.EnvClaudeCode && intent == "append") {',
     'TestProductionContextCarrierProfileNativeSuffixAndChannels/claude_code/false/append',
     'project selected prompt channels except Claude append'),
    ('V1', main, '\tlaunchContext := plan.Context(',
     '\tif frag.Revision == fragment.IdentityV2 && frag.Environment == fragment.EnvCodexCLI && decision.Mode == agentic.PermissionModeYolo { decision.Mode = agentic.PermissionModeNative }\n\tlaunchContext := plan.Context(',
     'TestProductionContextCarrierV2PermissionsUnchanged/codex_cli/false',
     'preserve permissions except v2 Codex direct yolo'),
    ('G1', main, '\tvar nativePolicy *execution.EffectiveNativePolicy',
     '\tif frag.Environment == fragment.EnvClaudeCode {\n\t\tfor i, entry := range admitted.Plan.Env { if entry == "CLAUDE_CODE_ENABLE_PROMPT_SUGGESTION=false" { admitted.Plan.Env[i] = "CLAUDE_CODE_ENABLE_PROMPT_SUGGESTION=true" } }\n\t}\n\tvar nativePolicy *execution.EffectiveNativePolicy',
     'TestProductionPipelineGoldens/claude_code/tracked=false',
     'preserve documented environment except Claude direct prompt suggestion value'),
    ('D1', root / 'go.mod', 'golang.org/x/sys v0.47.0', 'golang.org/x/sys v0.47.0',
     'TestConstructionDependencyReleasePin',
     'keep release pin token and same module bytes but admit a same-version replacement'),
])
rows = ['mutant\tnarrowing\ttest\texit\tverdict\tsurvival_bound']
failed = False
for ident, path, old, new, test, bound in mutants:
    original = path.read_bytes()
    try:
        source = original.decode()
        # N1's anchor appears twice since the §4.4b WithNetwork form
        # reuses the admitted-suffix shape; the replacement applies to
        # both and the named test still kills through the second site.
        expected = 2 if ident == 'N1' else 1
        if source.count(old) != expected:
            raise RuntimeError(f'{ident}: expected {expected} anchors, found {source.count(old)}')
        mutated = source.replace(old, new)
        if ident == 'D1':
            mutated += '\nreplace github.com/relux-works/skill-agents-management v0.5.48 => github.com/relux-works/skill-agents-management v0.5.48\n'
        path.write_text(mutated)
        mask = '^' + test.replace('/', '$/^') + '$'
        if ident == 'D1':
            # A configuration/token attack also runs the behavioral suite.
            mask += '|^TestProductionContextCarrier'
        command = ['go', 'test', './cmd/curator-run', '-count=1', '-v', '-run', mask]
        with (out / f'{ident}.log').open('w') as log:
            log.write('COMMAND: ' + ' '.join(command) + '\n'); log.flush()
            result = subprocess.run(command, cwd=root, env=dict(os.environ, GOWORK='off'), stdout=log, stderr=subprocess.STDOUT, timeout=90)
            log.write(f'\nEXIT: {result.returncode}\n')
        killed = result.returncode == 1 and f'--- FAIL: {test} ' in (out / f'{ident}.log').read_text(errors='replace')
        survival = 'none' if killed else bound + '; this test does not establish refusal of this member'
        rows.append(f'{ident}\t{bound}\t{test}\t{result.returncode}\t' + ('killed' if killed else 'SURVIVOR/unverified') + '\t' + survival)
        failed |= not killed
        print(rows[-1], flush=True)
    finally:
        path.write_bytes(original)
        if path.read_bytes() != original:
            raise RuntimeError(f'{ident}: restore mismatch')
(out / 'summary.tsv').write_text('\n'.join(rows) + '\n')
sys.exit(1 if failed else 0)
