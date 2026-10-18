#!/usr/bin/env python3
"""Bounded session-manager client/schema controls; restore exact candidate bytes."""
import argparse
import pathlib
import re
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('evidence', nargs='?', default='.temp/plugin-client-mutants')
parser.add_argument('--list', action='store_true')
args = parser.parse_args()
client = 'internal/sessionmanager/client/'
schemas = 'schemas/session-manager/'
# Each plant weakens one invariant and must fail its specific behavioral
# assertion. Compile errors, setup errors, timeouts and unrelated failures do
# not qualify. Every test also has an unmutated passing control.
mutants = [
 ('P01', client+'protocol.go', 'if exit != 0 {', 'if exit != 0 && false {', 'TestDecodeResponse/success-nonzero', 'wanted session_host_protocol_error'),
 ('P02', client+'protocol.go', 'if exit == 0 || managerschema.Validate', 'if false || managerschema.Validate', 'TestDecodeResponse/refusal-zero', 'wanted session_host_protocol_error'),
 ('P03', schemas+'validate.go', 'if len(data) > Limit {', 'if len(data) > Limit && false {', 'TestStrictDocuments', 'invalid document accepted'),
 ('P04', schemas+'validate.go', 'ok && !additional {', 'ok && !additional && false {', 'TestPublishedVectors/launch.json/unknown-member', 'valid=false, error=<nil>'),
 ('P05', schemas+'validate.go', 'n > hi {', 'n > hi && false {', 'TestPublishedVectors/launch.json', 'valid=false, error=<nil>'),
 ('P06', schemas+'validate.go', 'if branch, ok := s[key].(map[string]any); ok {', 'if branch, ok := s[key].(map[string]any); ok && false {', 'TestPublishedVectors/session.json/doorbell-invalid-21', 'valid=false, error=<nil>'),
 ('P07', schemas+'validate.go', 'if len(v.(string)) > 4096 {', 'if len(v.(string)) > 4096 && false {', 'TestPublishedVectors/launch.json', 'valid=false, error=<nil>'),
 ('P08', schemas+'child-env.schema.json', '^(TOKEN|SECRET|PASSWORD|PASSWD|API_KEY|APIKEY)$', '^(SECRET|PASSWORD|PASSWD|API_KEY|APIKEY)$', 'TestRequestPolicy/TOKEN', 'wanted secret_policy_violation'),
 ('P09', schemas+'child-env.schema.json', '^(sk-|ghp_|gho_', '^(sk-|gho_', 'TestRequestPolicy/VALUE', 'wanted secret_policy_violation'),
 ('P10', client+'transport_unix.go', 'if operation == "attach" {\n\t\tif v, ok := values["TERM"]', 'if true {\n\t\tif v, ok := values["TERM"]', 'TestEnvironmentProjection', 'projection mismatch'),
 ('P11', client+'transport_unix.go', 'p, err := canonicalPath(v)', 'p, err := v, error(nil)', 'TestEnvironmentProjection', 'projection mismatch'),
 ('P12', client+'transport_unix.go', 'Setpgid: true', 'Setpgid: false', 'TestScriptedRPC/success', 'RPC argv/group mismatch'),
 ('P13', client+'transport_unix.go', '\t\tb.cancel()\n', '\t\t// mutation: no overflow cancellation\n', 'TestCollectorsAndDeadlines', 'response overflow retained'),
 ('P14', client+'transport_unix.go', 'left := Limit - len(s.captured)', 'left := 3*Limit - len(s.captured)', 'TestCollectorsAndDeadlines', 'stderr not bounded/countable'),
 ('P15', client+'transport_unix.go', 'if err := markDescriptorsCloseOnExec(); err != nil {', 'if err := error(nil); err != nil {', 'TestRPCDescriptorIsolation', 'unrelated descriptor inherited'),
 ('P16', client+'capabilities.go', 'if !n.DelegatedTrial {', 'if !n.DelegatedTrial && false {', 'TestActivationMatrix/explicit0', 'wanted session_host_delegated_unauthorized'),
 ('P17', client+'capabilities.go', '!n.Detached && slices.Contains(operations, "launch-exact")', '!n.Detached && slices.Contains(d.Capabilities, "launch-exact/2")', 'TestNeededCapabilities', 'foreign runtime exact leaked'),
 ('P18', client+'capabilities.go', '!slices.Contains(d.Capabilities, cap) ||', '(!slices.Contains(d.Capabilities, cap) && false) ||', 'TestNeededCapabilities/missing-board-goal/1', 'wanted session_host_capability_missing'),
 ('P19', client+'reconcile_unix.go', 'if entry.Executable != p.Manager.Executable {', 'if entry.Executable != p.Manager.Executable && false {', 'TestScriptedSubmissionAndBinding/owner-changed', 'wanted werkstatt_operation_unresolved'),
 ('P20', client+'reconcile_unix.go', 'if err := tx.WritePending(p); err != nil {', 'if err := error(nil); err != nil {', 'TestScriptedSubmissionAndBinding/success', 'remote submission preceded durable correlation'),
 ('P21', client+'reconcile_unix.go', 'response, err = remote.Call(ctx, "cancel", data)', 'response, err = remote.Call(ctx, "cancel", data)\n\t\t\tresponse = Response{Outcome: &Outcome{Outcome: "unknown"}}', 'TestScriptedRecovery/prepared-race-started', 'session_liveness_unknown'),
 ('P22', client+'store_unix.go', 'if outcome != "started" && outcome != "failed" && outcome != "cancelled" {', 'if outcome != "started" && outcome != "failed" && outcome != "cancelled" && false {', 'TestStoreGuards', 'wanted werkstatt_record_invalid'),
 ('P23', client+'store_unix.go', 'if current != nil {', 'if current != nil && false {', 'TestStoreGuards', 'wanted werkstatt_busy'),
 ('P24', client+'store_unix.go', 'if actual.Generation != expected.Generation || !sameOperation(actual.OperationID, expected.OperationID) || !sameBinding(actual.Manager, expected.Manager) {', 'if (actual.Generation != expected.Generation || !sameOperation(actual.OperationID, expected.OperationID) || !sameBinding(actual.Manager, expected.Manager)) && false {', 'TestLivenessCAS/stale-operation', 'forbidden CAS'),
 ('P25', client+'store_unix.go', '} else if next.Generation != actual.Generation+1 {', '} else if next.Generation != actual.Generation+1 && false {', 'TestLivenessCAS/new-op-skipped-generation', 'forbidden CAS'),
 ('P26', client+'store_unix.go', 'if !sessionRead && next.OperationID != nil && a.OperationID == *next.OperationID {', 'if !sessionRead && next.OperationID != nil && a.OperationID == *next.OperationID && false {', 'TestLivenessCAS/abandoned', 'forbidden CAS'),
 ('P27', client+'reconcile_unix.go', 'if err := tx.clearTerminal(p.OperationID, outcome.Outcome); err != nil {', 'if err := error(nil); err != nil {', 'TestScriptedRecovery/already-started', 'want false'),
 ('P28', client+'protocol.go', 'record.OperationID != correlation.OperationID', '(record.OperationID != correlation.OperationID && false)', 'TestDecodeResponse/foreign-operation', 'wanted session_host_protocol_error'),
 ('P29', client+'transport_unix.go', 'if ctx.Err() != nil {', 'if ctx.Err() != nil && false {', 'TestScriptedRPC/group-timeout', 'deadline reason lost'),
 ('P30', schemas+'validate.go', 'case "date-time":', 'case "date-time":\n\t\t\t\treturn nil', 'TestPublishedVectors/session.json/date-time-invalid-day', 'valid=false, error=<nil>'),
 ('P31', client+'transport_unix.go', 'if ctx.Err() != nil || output.overflow || errors.Is(err, exec.ErrWaitDelay) {', 'if true {', 'TestSuccessfulRPCPreservesPluginChild', 'successful RPC killed plugin child'),
 ('P32', schemas+'list-page.schema.json', 'urn:relux:curator-run:session-record', 'urn:relux:curator-run:unpublished', 'TestSchemaCatalog', 'unresolved reference'),
 ('P33', client+'reconcile_unix.go', 'if err != nil {\n\t\t\tresultErr = refusal("werkstatt_liveness_write_failed", map[string]string{"session": result.Session})', 'if err != nil {\n\t\t\tresultErr = err', 'TestTerminalLivenessFailure/corrupt-storage', 'wanted werkstatt_liveness_write_failed'),
 ('P34', schemas+'refusal.schema.json', '"const": "werkstatt_busy"', '"enum": ["werkstatt_busy", "werkstatt_identity_missing"]', 'TestDecodeResponse/launcher-local-refusal', 'wanted session_host_protocol_error'),
 ('P35', schemas+'outcome.schema.json', '"plan_refused",', '"plan_refused", "werkstatt_record_invalid",', 'TestDecodeResponse/launcher-local-history', 'wanted session_host_protocol_error'),
 ('P36', schemas+'session-record.schema.json', '"const": "unknown"', '"enum": ["unknown", "down"]', 'TestPublishedVectors/session-record.json/unreadable-forbidden-state', 'valid=false, error=<nil>'),
 ('P37', schemas+'launch.schema.json', '[0-9]{2}Z$', '[0-9]{2}(\\\\.[0-9]+)?Z$', 'TestPublishedVectors/launch.json/fractional-timestamp', 'valid=false, error=<nil>'),
 ('P38', schemas+'outcome.schema.json', '"readable": {\n                  "const": true', '"readable": {\n                  "enum": [true, false]', 'TestPublishedVectors/outcome.json/unreadable-started', 'valid=false, error=<nil>'),
 ('P39', client+'reconcile_unix.go', '.Format(time.RFC3339)', '.Format(time.RFC3339Nano)', 'TestScriptedSubmissionAndBinding/fractional-clock', 'fractional clock prevented liveness write'),
]
if args.list:
    for ident, path, _, _, test, _ in mutants:
        print(ident, path, test)
    sys.exit(0)
out = (root / args.evidence).resolve()
out.mkdir(parents=True, exist_ok=True)
rows = ['mutant\ttest\texit\tverdict']
failed = False
for ident, relative, old, new, test, assertion in mutants:
    path = root / relative
    original = path.read_bytes()
    try:
        source = original.decode()
        # The two reconciliation loops share the cancel statement; weaken only
        # the durable loop under test, leaving plain reconciliation intact.
        count = source.count(old)
        if count != (2 if ident == 'P21' else 1):
            raise RuntimeError(f'{ident}: anchor count {count}')
        path.write_text(source.replace(old, new, 1))
        package = './schemas/session-manager' if test.startswith(('TestPublished', 'TestStrict', 'TestSchemaCatalog')) else './internal/sessionmanager/client'
        pattern = '/'.join('^' + re.escape(part) + '$' for part in test.split('/'))
        command = ['go', 'test', package, '-count=1', '-v', '-timeout=45s', '-run', pattern]
        with (out / f'{ident}.log').open('w') as log:
            log.write('COMMAND: ' + ' '.join(command) + '\n'); log.flush()
            result = subprocess.run(command, cwd=root, stdout=log, stderr=subprocess.STDOUT, timeout=60)
            log.write(f'\nEXIT: {result.returncode}\n')
        evidence = (out / f'{ident}.log').read_text(errors='replace')
        killed = result.returncode == 1 and f'--- FAIL: {test} ' in evidence and assertion in evidence
        rows.append(f'{ident}\t{test}\t{result.returncode}\t' + ('killed' if killed else 'SURVIVOR/unverified'))
        failed |= not killed
        print(rows[-1], flush=True)
    finally:
        path.write_bytes(original)
        if path.read_bytes() != original:
            raise RuntimeError(f'{ident}: restore mismatch')
(out / 'summary.tsv').write_text('\n'.join(rows) + '\n')
sys.exit(1 if failed else 0)
