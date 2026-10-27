#!/usr/bin/env python3
"""Narrow carrier engagement and entry gates; require named behavioral failures."""
import hashlib
import json
import os
import pathlib
import re
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
# Only the source listing above uses canonical Git routing. Go and its fixture
# children must never inherit routing or configuration for that repository.
env = os.environ.copy()
for key in list(env):
    if key in ('GIT_DIR', 'GIT_WORK_TREE', 'GIT_INDEX_FILE', 'GIT_CONFIG_COUNT',
               'GIT_CONFIG_PARAMETERS', 'GIT_COMMON_DIR', 'GIT_OBJECT_DIRECTORY',
               'GIT_ALTERNATE_OBJECT_DIRECTORIES') or key.startswith(('GIT_CONFIG_KEY_', 'GIT_CONFIG_VALUE_')):
        env.pop(key, None)
env.update(GIT_CONFIG_GLOBAL='/dev/null', GIT_CONFIG_NOSYSTEM='1', GOWORK='off')

# Ask Go for the selected module, rather than planting an obsolete require token.
module = 'github.com/relux-works/skill-agents-management'
selected = json.loads(subprocess.check_output(
    ['go', 'list', '-m', '-json', module], cwd=root, env=env, timeout=300))
if selected.get('Replace') or not selected.get('Version'):
    raise RuntimeError('positive dependency baseline must select an unreplaced release')
version = selected['Version']
(out / 'selected-module.json').write_text(json.dumps(selected, indent=2) + '\n')


def run_test(label, mask):
    # Cold-cache compilation is included in this bounded five-minute budget.
    command = ['go', 'test', './cmd/curator-run', '-count=1', '-timeout=4m', '-v', '-run', mask]
    with (out / f'{label}.log').open('w') as log:
        log.write('COMMAND: ' + ' '.join(command) + '\n'); log.flush()
        try:
            result = subprocess.run(command, cwd=root, env=env, stdout=log,
                                    stderr=subprocess.STDOUT, timeout=300)
        except subprocess.TimeoutExpired:
            log.write('\nEXIT: timeout (not a killed mutant)\n')
            raise
        log.write(f'\nEXIT: {result.returncode}\n')
    return result.returncode, (out / f'{label}.log').read_text(errors='replace')


plan = root / 'internal/plan/plan.go'
cli = root / 'internal/cli/cli.go'
composition = root / 'internal/composition/composition.go'
mutants = []
for ident, runtime, flag, channel, test in [
    ('C1', 'claude', '--mcp-config', 'MCP', 'claude_code/--mcp-config/false'),
    ('C2', 'claude', '--system-prompt-file=other.md', 'SystemPrompt', 'claude_code/--system-prompt-file/false'),
    ('C3', 'codex', '--profile', 'MCP', 'codex_cli/--profile/false'),
    ('C4', 'codex', '-c', 'SystemPrompt', 'codex_cli/-c/false'),
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
                'TestProductionContextCarrierRefusesAdmittedNativeSuffixDrift/false',
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
     'TestProductionPipelineGoldens/claude_code/native/off',
     'project selected prompt channels except Claude append'),
    ('V1', main, '\tlaunchContext := plan.Context(',
     '\tif frag.Revision == fragment.IdentityV2 && frag.Environment == fragment.EnvCodexCLI && decision.Mode == agentic.PermissionModeYolo { decision.Mode = agentic.PermissionModeNative }\n\tlaunchContext := plan.Context(',
     'TestProductionContextCarrierV2PermissionsUnchanged/codex_cli/false',
     'preserve permissions except v2 Codex direct yolo'),
    ('G1', main, '\treturn runNative(value, decision, inspection, stdout, stderr, deps, boundary)',
     '\tif frag.Environment == fragment.EnvClaudeCode {\n\t\tfor i, entry := range value.Env { if entry == "CLAUDE_CODE_ENABLE_PROMPT_SUGGESTION=false" { value.Env[i] = "CLAUDE_CODE_ENABLE_PROMPT_SUGGESTION=true" } }\n\t}\n\treturn runNative(value, decision, inspection, stdout, stderr, deps, boundary)',
     'TestProductionPipelineGoldens/claude_code/native/off',
     'preserve documented environment except Claude direct prompt suggestion value'),
    ('D1', root / 'go.mod', module + ' ' + version, module + ' ' + version,
     'TestConstructionDependencyReleasePin',
     'keep release pin token and same module bytes but admit a same-version replacement'),
])
# Keep the prompt-drop plant on the argv golden: the carrier-inspection leaf
# dereferences the deliberately missing prompt and would only prove a panic.
positive_logs = ''
for label, mask in [
    ('positive-carrier', '^(TestProductionContextCarrier.*|TestProductionDeprecatedInfraAliasesRefused|TestConstructionDependencyReleasePin)$'),
    ('positive-native', '^TestProductionPipelineGoldens$/^claude_code$/^native$/^off$'),
]:
    code, text = run_test(label, mask)
    if code != 0:
        raise RuntimeError(f'{label}: positive baseline exited {code}')
    positive_logs += text
for _, _, _, _, test, _ in mutants:
    if f'--- PASS: {test} ' not in positive_logs:
        raise RuntimeError(f'positive baseline did not execute {test}')

# A named failure alone is insufficient: require the intended assertion and
# reject crashes, missing leaves, build failures and unrelated failing tests.
assertions = {
    **{ident: 'exit=0' for ident in ('C1', 'C2', 'C3', 'C4', 'N1')},
    'A1': 'exit=1', 'A2': 'exit=1',
    'R1': 'path_prepend has no registered root-bound launch mapping',
    'P1': 'pipeline-claude_code-false mismatch',
    'V1': 'request=', 'G1': 'pipeline-claude_code-false mismatch',
    'D1': f'release module must be {version} without replacement: version={version} replaced=true',
}
restoration = []
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
            mutated += f'\nreplace {module} {version} => {module} {version}\n'
        path.write_text(mutated)
        mask = '^' + test.replace('/', '$/^') + '$'
        if ident == 'D1':
            # The replacement guard must fail while carrier behavior still passes.
            mask += '|^TestProductionContextCarrier'
        code, text = run_test(ident, mask)
        failures = re.findall(r'--- FAIL: (\S+) ', text)
        killed = (code == 1 and test in failures and assertions[ident] in text
                  and all(test == name or test.startswith(name + '/') for name in failures)
                  and not any(marker in text for marker in (
                      'panic:', 'fatal error:', '[build failed]', '[no tests to run]', 'setup failed')))
        survival = 'none' if killed else bound + '; this test does not establish refusal of this member'
        rows.append(f'{ident}\t{bound}\t{test}\t{code}\t' + ('killed' if killed else 'SURVIVOR/unverified') + '\t' + survival)
        failed |= not killed
        print(rows[-1], flush=True)
    finally:
        path.write_bytes(original)
        restored = path.read_bytes()
        if restored != original:
            raise RuntimeError(f'{ident}: restore mismatch')
        restoration.append(dict(mutant=ident, anchors=expected,
                                original_sha256=hashlib.sha256(original).hexdigest(),
                                restored_sha256=hashlib.sha256(restored).hexdigest()))
        (out / 'restoration.json').write_text(json.dumps(restoration, indent=2) + '\n')
(out / 'summary.tsv').write_text('\n'.join(rows) + '\n')
sys.exit(1 if failed else 0)
