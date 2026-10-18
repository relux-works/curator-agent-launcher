#!/usr/bin/env python3
"""Bounded registry/registration negative controls; restore candidate bytes."""
import pathlib
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parents[1]
out = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else root / '.temp/session-manager-mutants').resolve()
out.mkdir(parents=True, exist_ok=True)
registry = root / 'internal/sessionmanager/registry.go'
service = root / 'internal/sessionmanager/service.go'
transaction = root / 'internal/sessionmanager/transaction.go'
transaction_unix = root / 'internal/sessionmanager/transaction_unix.go'
atomic = root / 'internal/sessionmanager/atomic.go'
mutants = [
 ('R01', registry, 'schema != Schema', 'schema != Schema && schema != "other"', 'TestRegistryClosedSchema/schema'),
 ('R02', registry, 'if !allowed[member.Key] {', 'if !allowed[member.Key] && member.Key != "extra" {', 'TestRegistryClosedSchema/root-extra'),
 ('R03', registry, 'return idPattern.MatchString(id)', 'return idPattern.MatchString(id) || id == "Bad"', 'TestRegistryClosedSchema/bad-id'),
 ('R04', registry, 'entry.Protocol != Protocol', 'entry.Protocol != Protocol && entry.Protocol != "other"', 'TestRegistryClosedSchema/protocol'),
 ('R05', registry, 'len(prefix.Arr) > 4', 'len(prefix.Arr) > 5', 'TestRegistryClosedSchema/prefix-count'),
 ('R06', registry, '^[ -~]{1,128}$', '^[ -~]{1,129}$', 'TestRegistryGrammarBoundaries'),
 ('R07', registry, '[]configfile.Layer{configfile.System, configfile.User}', '[]configfile.Layer{configfile.User, configfile.System}', 'TestRegistryLayersAndSecurity/new'),
 ('R08', registry, 'return state, &Error{Code: CodeHostMissing, Reason: "registered executable missing"}', 'return State{Kind: "empty"}, nil', 'TestRegistryStatesAndNoPATH/absent'),
 ('R09', service, 'case c.NoSelect:', 'case c.NoSelect: registry.Selected = id', 'TestRegistrationRules/first-no-select'),
 ('R10', service, 'case exists:', 'case exists: registry.Selected = id', 'TestExecutableChangeAndManifestStdin/keep'),
 ('R11', service, 'strings.EqualFold(strings.TrimSpace(answer), "yes")', 'strings.EqualFold(strings.TrimSpace(answer), "yes") || strings.TrimSpace(answer) == ""', 'TestRegistrationRules/peer-tty-default'),
 ('R12', service, 'notice += fmt.Sprintf("registered %s;', 'registry.Selected = id; notice += fmt.Sprintf("registered %s;', 'TestRegistrationRules/peer-non-tty'),
 ('R13', service, 'if present {\n\t\treturn &Error{Code: CodeShadowed, Path: path}', 'if present && false {\n\t\treturn &Error{Code: CodeShadowed, Path: path}', 'TestMutationLayers/register'),
 ('R14', service, 'if c.Select && c.NoSelect {', 'if c.Select && c.NoSelect && false {', 'TestManifestRefusalsPreserveRegistry/flags'),
 ('R15', service, 'if registry.Selected == c.ID {', 'if registry.Selected == c.ID && len(registry.Managers) == 0 {', 'TestSelectAndUnregister'),
 ('R16', atomic, 'if hooks.beforeRename != nil {', 'if hooks.beforeRename != nil && false {', 'TestAtomicFailurePreservesOldFile/before-rename'),
 ('R17', atomic, 'if err := sync(dir); err != nil {', 'if err := sync(dir); err != nil && false {', 'TestAtomicFailurePreservesOldFile/directory-sync'),
 ('R18', atomic, 'restoreErr = os.Rename(rollback, path)', 'restoreErr = nil', 'TestAtomicFailurePreservesOldFile/directory-sync'),
 ('R19', service, 'lock, err := acquireRegistryLock(path, mode)', 'lock, err := os.Open(os.DevNull)', 'TestOverlappingRegistryMutations/register-register/overlap'),
 ('R20', service, 'defer lock.Close()', 'lock.Close(); defer lock.Close()', 'TestOverlappingRegistryMutations/rollback-register/overlap'),
 ('R21', service, 'if errors.Is(err, os.ErrDeadlineExceeded) {\n\t\t\t\treturn &Error{Code: "usage", Err: err}', 'if errors.Is(err, os.ErrDeadlineExceeded) {\n\t\t\t\treturn &Error{Code: CodeInvalid, Err: err}', 'TestRegistryPromptDeadline/silent'),
 ('R22', transaction, 'return nil, &Error{Code: CodeBusy}', 'return nil, &Error{Code: CodeInvalid}', 'TestRegistryLockBusyAndRecovery'),
 ('R23', transaction_unix, 'deadline := time.Now().Add(timeout)', 'deadline := time.Now().Add(timeout + 2*time.Second)', 'TestRegistryPromptDeadline/silent'),
 ('R24', transaction_unix, 'raw, err := f.SyscallConn()', '_ = f.Fd(); raw, err := f.SyscallConn()', 'TestRegistryPromptDeadline/yes'),
]
rows = ['mutant\ttest\texit\tverdict']
failed = False
for ident, path, old, new, test in mutants:
    original = path.read_bytes()
    try:
        source = original.decode()
        if source.count(old) != 1:
            raise RuntimeError(f'{ident}: anchor count {source.count(old)} != 1')
        path.write_text(source.replace(old, new, 1))
        command = ['go', 'test', './internal/sessionmanager', '-count=1', '-v', '-run', '^' + test.replace('/', '$/^') + '$']
        with (out / f'{ident}.log').open('w') as log:
            log.write('COMMAND: ' + ' '.join(command) + '\n'); log.flush()
            result = subprocess.run(command, cwd=root, stdout=log, stderr=subprocess.STDOUT, timeout=60)
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
