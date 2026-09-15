#!/usr/bin/env python3
"""SPEC 4.3 defaults and lineup narrowing probes; restores candidate bytes.

Each mutant weakens one gate to admit exactly one member of the class the
gate must reject, and the named test must fail. A delete-only mutant is not
accepted as evidence. Optional env DEFAULTS_MUTANT_IDS limits the run to a
comma-separated subset of mutant ids, for bounded sequential execution.
"""
import os
import pathlib
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parents[1]
out = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else root / '.temp/defaults-mutants').resolve()
out.mkdir(parents=True, exist_ok=True)
source = root / 'internal/defaults/defaults.go'
lineup = root / 'internal/defaults/lineup.go'
main = root / 'cmd/curator-run/main.go'
json = root / 'internal/fragment/json.go'
pkg_defaults = './internal/defaults'
pkg_main = './cmd/curator-run'
mutants = [
 ('schema-version', source, 'm.Value.Str != Schema', '(m.Value.Str != Schema && m.Value.Str != "v2")', 'TestLoadRejectsInvalid/wrong-schema', 'admit schema v2 only'),
 ('locked-null', source, 'm.Value.Kind != fragment.KindBool', 'm.Value.Kind != fragment.KindBool && m.Value.Kind != fragment.KindNull', 'TestLoadRejectsInvalid/locked-null', 'admit null lock only'),
 ('defaults-null', source, 'm.Value.Kind != fragment.KindObject', 'm.Value.Kind != fragment.KindObject && m.Value.Kind != fragment.KindNull', 'TestLoadRejectsInvalid/defaults-null', 'admit null defaults only'),
 ('unknown-env', source, 'fragment.HomeVariable(env.Key) == ""', 'fragment.HomeVariable(env.Key) == "" && env.Key != "future"', 'TestLoadRejectsInvalid/unknown-env', 'admit future env only'),
 ('empty-entry', source, 'len(env.Value.Obj) == 0', '(len(env.Value.Obj) == 0 && env.Key != "pi")', 'TestLoadRejectsInvalid/empty-entry', 'admit empty pi entry only'),
 ('entry-null', source, 'env.Value.Kind != fragment.KindObject || len(env.Value.Obj) == 0', '(env.Value.Kind != fragment.KindObject || len(env.Value.Obj) == 0) && env.Value.Kind != fragment.KindNull', 'TestLoadRejectsInvalid/null-entry', 'admit null env entry only'),
 ('member-null', source, 'member.Value.Kind != fragment.KindString', 'member.Value.Kind != fragment.KindString && member.Value.Kind != fragment.KindNull', 'TestLoadRejectsInvalid/model-null', 'admit null string members only'),
 ('extra-member', source, 'return file{}, fmt.Errorf("unknown member %s.%s", env.Key, member.Key)', 'if member.Key != "extra" { return file{}, fmt.Errorf("unknown member %s.%s", env.Key, member.Key) }', 'TestLoadRejectsInvalid/unknown-member', 'admit extra entry member only'),
 ('sig', source, 'return file{}, fmt.Errorf("unknown member %q", m.Key)', 'if m.Key != "sig" { return file{}, fmt.Errorf("unknown member %q", m.Key) }', 'TestLoadRejectsInvalid/unknown-sig', 'admit top-level sig only; token retained'),
 ('required-schema', source, 'if _, ok := root.Get(key); !ok {', 'if _, ok := root.Get(key); !ok && key != "schema" {', 'TestLoadRejectsInvalid/missing-schema', 'allow absent schema only'),
 ('required-defaults', source, 'if _, ok := root.Get(key); !ok {', 'if _, ok := root.Get(key); !ok && key != "defaults" {', 'TestLoadRejectsInvalid/missing-defaults', 'allow absent defaults only'),
 ('duplicate', json, 'if seen[key] {', 'if seen[key] && key != "model" {', 'TestLoadRejectsInvalid/duplicate-model', 'admit duplicate model only in strict JSON reader'),
 ('broken-link', source, 'return fmt.Errorf("unreadable symlink %s: %v", path, err)', 'if os.IsNotExist(err) { return err }; return fmt.Errorf("unreadable symlink %s: %v", path, err)', 'TestLoadFilesystem/broken-link', 'misclassify ENOENT symlink targets only as absence'),
 ('permission', source, 'data, err := os.ReadFile(path)\n\tif err != nil {', 'data, err := os.ReadFile(path)\n\tif os.IsPermission(err) { return file{}, nil }; if err != nil {', 'TestLoadFilesystem/read-permission', 'treat permission-denied reads only as absence'),
 ('directory', source, 'if !info.Mode().IsRegular() {', 'if info.IsDir() { return file{}, nil }; if !info.Mode().IsRegular() {', 'TestLoadFilesystem/directory', 'treat directories only as absence'),
 ('resolve-env', source, 'fragment.HomeVariable(environment) == ""', 'fragment.HomeVariable(environment) == "" && environment != "future"', 'TestLoadKnownEnvironmentsAndPresence', 'admit future resolve environment only'),
 ('model-lock', source, 'locked && row.flag.Present && row.machine.Present', 'locked && row.flag.Present && row.machine.Present && row.name != "model"', 'TestResolveLocks/model', 'enforce locks only for effort'),
 ('effort-lock', source, 'locked && row.flag.Present && row.machine.Present', 'locked && row.flag.Present && row.machine.Present && row.name != "effort"', 'TestResolveLocks/effort', 'enforce locks only for model'),
 ('operator-ignore', source, 'if locked {\n\t\toperator = Pair{}', 'if locked && machine.Effort.Present {\n\t\toperator = Pair{}', 'TestResolveLocks/model', 'ignore operator only when machine sets effort'),
 ('presence', source, 'if candidate.Present {', 'if candidate.Present && !(candidate.Origin == OriginFlag && candidate.Value == "") {', pkg_defaults, 'TestResolveExplicitEmptyOverrides', 'ignore explicitly empty flags only'),
 ('lineup-bottom', lineup, 'top := vendorplugin.Lineup(models)[0].Model', 'top := vendorplugin.Lineup(models)[len(models)-1].Model', pkg_defaults, 'TestCompleteLineupTops', 'supply bottom row only'),
 ('effort-none-fill', lineup, 'if row.Effort.Support != agentic.EffortSupportRequired {', 'if row.Effort.Support != agentic.EffortSupportRequired && row.ID != "claude-haiku-4-5" {', pkg_main, 'TestRunDefaultsPerMember/no-effort', 'admit effort only for the effortless haiku row'),
 ('effort-overwrite-explicit', lineup, 'if partial.Model.Present && partial.Effort.Present {', 'if partial.Model.Present && partial.Effort.Present && partial.Effort.Value != "low" {', pkg_main, 'TestRunDefaultsPerMember/operator-model-machine-effort', 'overwrite explicitly supplied low only'),
 ('top-effort-overwrite', lineup, 'if partial.Effort.Present {\n\t\treturn out, nil\n\t}', 'if partial.Effort.Present && partial.Effort.Value != "unsupported-word" {\n\t\treturn out, nil\n\t}', pkg_main, 'TestRunDefaultsPerMember/flag-effort', 'overwrite unsupported-word only'),
 ('unresolvable-admit-empty', lineup, 'if len(cands) == 0 {', 'if len(cands) == 0 && environment == "claude_code" { return Resolved{}, nil }; if len(cands) == 0 {', pkg_defaults, 'TestCompleteWrongVendorModels', 'admit empty claude_code candidates only'),
 ('ambiguity-model-admit', lineup, '\t\tif len(matches) > 1 {', '\t\tif len(matches) > 2 {', pkg_defaults, 'TestCompleteAmbiguousRuntime/configured-model', 'admit exactly-two-contributor models'),
 ('top-contributor-admit', lineup, '\tif len(matches) != 1 {', '\tif len(matches) != 1 && len(matches) != 2 {', pkg_defaults, 'TestCompleteAmbiguousRuntime/lineup-top', 'admit exactly two top contributors'),
 ('fire-past-resolve-error', lineup, 'partial, err := f.Resolve(environment, flags)\n\tif err != nil {', 'partial, err := f.Resolve(environment, flags)\n\tif err != nil && environment != "pi" {', pkg_main, 'TestRunDefaultsFailuresStopBeforeGroup/model-lock', 'ignore Pi lock refusal only'),
 ('mapping-skip', lineup, 'target, err := mapping.Resolve(environment)\n\tif err != nil {', 'target, err := mapping.Resolve(environment)\n\tif err != nil && environment != "opencode" {', pkg_defaults, 'TestCompleteMappingRefusal', 'skip opencode mapping refusal only'),
 ('findrow-fuzzy', lineup, 'if string(cand.model.ID) == id {', 'if string(cand.model.ID) == id || (id == "nope-not-a-model" && cand.model.ID == "claude-opus-5") {', pkg_defaults, 'TestCompleteNoEffortSemantics/unknown-model', 'admit one unknown id while preserving all exact matches'),
 ('origin-mislabel', lineup, 'out.Model = ResolvedMember{Member: Member{Value: string(top.ID), Present: true}, Origin: OriginLineup}', 'out.Model = ResolvedMember{Member: Member{Value: string(top.ID), Present: true}, Origin: OriginFlag}', pkg_defaults, 'TestCompleteLineupTops', 'label lineup values as flags'),
 ('runtime-drop', lineup, '\t\tif matches := contributorsFor(cands, partial.Model.Value); len(matches) == 1 {\n\t\t\tout.Runtime = string(matches[0].runtime)\n\t\t}\n\t\treturn out, nil', '\t\tif matches := contributorsFor(cands, partial.Model.Value); len(matches) == 1 {\n\t\t\tout.Runtime = ""\n\t\t}\n\t\treturn out, nil', pkg_defaults, 'TestCompletePerMemberFill', 'drop the recorded binding'),
 ('exit-usage-downgrade', main, 'return diagnostics.ExitForCode(derr.Code)', 'return diagnostics.ExitOperational', pkg_main, 'TestRunDiagnosticsContract/usage_locked_flag', 'downgrade usage to exit 1'),
 ('resolution-failure-admit', lineup, 'runtime, err := reg.ResolveRuntime(decl.ID)\n\t\tif err != nil {\n\t\t\treturn nil, err', 'runtime, err := reg.ResolveRuntime(decl.ID)\n\t\tif err != nil {\n\t\t\tif decl.ID == "claude-bad" { continue }; return nil, err', pkg_defaults, 'TestCompleteRuntimeResolutionFailure/partial-failure', 'skip one broken declaration only'),
 ('pi-union-leak', lineup, 'if string(cand.runtime) == id {', 'if string(cand.runtime) == id || cand.runtime == "pi-openai" {', pkg_main, 'TestRunPiRuntimePreference/ordered-not-declaration-or-score', 'admit openai rows into preferred anthropic lineup'),
 ('pi-empty-runtime', lineup, 'if len(preferred) > 0 {', 'if len(preferred) > 0 || id == "pi-anthropic" {', pkg_main, 'TestRunPiRuntimePreference/first-with-no-driven-rows', 'stop at an empty anthropic runtime only'),
 ('pi-preference-order', lineup, 'const piRuntimePreference = "pi-anthropic pi-openai pi-google"', 'const piRuntimePreference = "pi-openai pi-anthropic pi-google"', pkg_main, 'TestRunPiRuntimePreference/ordered-not-declaration-or-score', 'swap only first two preferred runtimes'),
 ('pi-configured-restrict', lineup, 'if target.System == "pi-native" && !partial.Model.Present {', 'if target.System == "pi-native" && (!partial.Model.Present || partial.Model.Value == "gpt-5.6-sol") {', pkg_main, 'TestRunDefaultsPerMember/pi-openai-override', 'restrict configured OpenAI model to preferred vendor only'),
 ('pi-unpreferred-admit', lineup, 'if len(preferred) == 0 {', 'if len(preferred) == 0 && len(cands) > 0 && cands[0].runtime == \"pi-other\" { preferred = cands }; if len(preferred) == 0 {', pkg_main, 'TestRunPiRuntimePreference/unpreferred-only', 'admit one unpreferred runtime only'),
 ('path-failure-as-absence', main, 'paths, err := deps.configPaths()', 'paths, err := deps.configPaths(); if err != nil && err.Error() == "path discovery failed" { paths = deps.defaults; err = nil }', pkg_main, 'TestRunDefaultsPathOrdering/path-failure', 'treat one configuration discovery error as absent paths'),
 ('nil-registry-admit-claude', lineup, 'if reg == nil {', 'if reg == nil && environment == "claude_code" { return Resolved{}, nil }; if reg == nil {', pkg_defaults, 'TestCompleteNilRegistry', 'admit nil registry only for claude_code'),
 ('cross-driven-row', lineup, 'if model.DrivenBy(agentic.SystemID(system)) {', 'if model.DrivenBy(agentic.SystemID(system)) || model.ID == "gpt-6-astra" {', pkg_defaults, 'TestCompleteWrongVendorModels', 'admit the unsupported astra row only'),
]
rows = ['mutant\tnarrows_to\ttest\texit\tverdict\tsurvival_bound']
failed = False
wanted = os.environ.get('DEFAULTS_MUTANT_IDS')
if wanted:
    keep = {ident.strip() for ident in wanted.split(',') if ident.strip()}
    mutants = [mutant for mutant in mutants if mutant[0] in keep]
    if not mutants:
        raise RuntimeError(f'no mutant matches DEFAULTS_MUTANT_IDS={wanted!r}')
for mutant in mutants:
    # Original file/flag probes predate the per-mutant package field and
    # always ran the defaults suite; normalize them to the same shape.
    if len(mutant) == 6:
        ident, path, old, new, test, bound = mutant
        pkg = pkg_defaults
    else:
        ident, path, old, new, pkg, test, bound = mutant
    original = path.read_bytes()
    try:
        text = original.decode()
        if text.count(old) != 1:
            raise RuntimeError(f'{ident}: anchor is not unique')
        path.write_text(text.replace(old, new, 1))
        command = ['go', 'test', pkg, '-run', '^' + test + '$', '-count=1', '-v']
        with (out / f'{ident}.log').open('w') as log:
            log.write('COMMAND: ' + ' '.join(command) + '\n')
            log.flush()
            result = subprocess.run(command, cwd=root, stdout=log, stderr=subprocess.STDOUT, timeout=120)
            log.write(f'\nEXIT: {result.returncode}\n')
        killed = result.returncode == 1 and f'--- FAIL: {test} ' in (out / f'{ident}.log').read_text()
        rows.append(f'{ident}\t{bound}\t{test}\t{result.returncode}\t' + ('killed\tnone' if killed else f'SURVIVOR/unverified\t{bound} is not proven'))
        failed |= not killed
    finally:
        path.write_bytes(original)
        assert path.read_bytes() == original
(out / 'summary.tsv').write_text('\n'.join(rows) + '\n')
print('\n'.join(rows))
sys.exit(1 if failed else 0)
