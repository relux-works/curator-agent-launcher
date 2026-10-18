#!/usr/bin/env python3
"""Bounded doorbell controls; restore exact candidate bytes after each plant."""
import argparse
import pathlib
import re
import subprocess
import sys
root = pathlib.Path(__file__).resolve().parents[1]
p = argparse.ArgumentParser(description=__doc__)
p.add_argument('evidence', nargs='?', default='.temp/doorbell-mutants')
p.add_argument('--list', action='store_true')
a = p.parse_args()
hooks = 'internal/doorbell/hooks_unix.go'
controls = [
 ('B01', hooks, 'if !verify(data, identity, executable) {', 'if false && !verify(data, identity, executable) {', './cmd/curator-run', 'TestDoorbellBinary/hooks/missing-stop', 'hook exit=0 want=1'),
 ('B02', hooks, 'if forbidden {', 'if forbidden && false {', './internal/doorbell', 'TestTrustedResolution/git-dir', 'trust verdict=true want=false'),
 ('B03', hooks, 'if !filepath.IsAbs(dir) {', 'if !filepath.IsAbs(dir) && false {', './internal/doorbell', 'TestTrustedResolution/relative', 'trust verdict=true want=false'),
 ('B04', hooks, '|| agent != identity', '|| false', './cmd/curator-run', 'TestDoorbellBinary/hooks/wrong-agent', 'hook exit=0 want=1'),
 ('B05', hooks, 'hook.Type != "command" ||', 'false ||', './cmd/curator-run', 'TestDoorbellBinary/hooks/wrong-type', 'hook exit=0 want=1'),
 ('B06', hooks, 'len(words) != 7', 'len(words) < 7', './cmd/curator-run', 'TestDoorbellBinary/hooks/extra-argument', 'hook exit=0 want=1'),
 ('B07', hooks, 'path != executable', '(path != executable && false)', './cmd/curator-run', 'TestDoorbellBinary/hooks/wrong-executable', 'hook exit=0 want=1'),
 ('B08', 'cmd/curator-run/host_selection.go', 'Doorbell: inv.A2A != ""', 'Doorbell: false', './cmd/curator-run', 'TestDoorbellBinary/plain/missing-capability', 'want calls=[describe]'),
 ('B09', 'internal/sessionmanager/client/capabilities.go', '!n.Detached && !n.Doorbell &&', '!n.Detached &&', './internal/sessionmanager/client', 'TestDoorbellRoute', 'doorbell route=exact'),
 ('B10', 'internal/sessionmanager/client/doorbell.go', 'if d.Reason != nil {', 'if d.Reason != nil && false {', './cmd/curator-run', 'TestDoorbellBinary/surface/unsupported_version', 'doorbell line mismatch:'),
 ('B11', 'cmd/curator-run/delegated.go', '"heartbeat": "1m", "max_wakes_per_hour": 6', '"heartbeat": "2m", "max_wakes_per_hour": 6', './cmd/curator-run', 'TestDoorbellBinary/plain/success', 'default doorbell lost'),
 ('B12', 'cmd/curator-run/werkstatt_unix.go', 'r.Runtime == fragment.EnvClaudeCode && r.Doorbell != nil', 'r.Runtime == fragment.EnvClaudeCode && r.Doorbell != nil && false', './cmd/curator-run', 'TestDoorbellBinary/hooks/fail', 'hook exit=0 want=1'),
 ('B13', hooks, 'if !safeSettings(workspace) {\n\t\treturn failure("install")', 'if false && !safeSettings(workspace) {\n\t\treturn failure("install")', './cmd/curator-run', 'TestDoorbellBinary/settings-path/outside-symlink/planted', 'settings plant admitted'),
 ('B14', hooks, 'stat.Nlink == 1', 'stat.Nlink >= 1', './cmd/curator-run', 'TestDoorbellBinary/hooks/post-hard-link', 'hook exit=0 want=1'),
 ('B15', hooks, 'info.Mode().Perm()&0022 != 0', '(info.Mode().Perm()&0022 != 0 && false)', './cmd/curator-run', 'TestDoorbellBinary/settings-path/unsafe-directory/planted', 'settings plant admitted'),
 ('B16', hooks, 'if inside(i.selected, root) || inside(i.executable, root) {', 'if (inside(i.selected, root) || inside(i.executable, root)) && false {', './internal/doorbell', 'TestTrustedResolution/trusted', 'frozen helper entered another workspace'),
]
if a.list:
 for ident, path, _, _, _, test, _ in controls: print(ident, path, test)
 sys.exit(0)
out = (root/a.evidence).resolve();out.mkdir(parents=True,exist_ok=True)
rows=['control\ttest\texit\tverdict'];failed=False
for ident, relative, old, new, package, test, assertion in controls:
 path=root/relative;original=path.read_bytes()
 try:
  source=original.decode()
  if source.count(old)!=1: raise RuntimeError(f'{ident}: anchor count {source.count(old)}')
  source=source.replace(old,new,1)
  # The extra-argument plant must weaken both length and suffix validation.
  if ident=='B06':
   anchor='strings.Join(words[1:], " ")'
   if source.count(anchor)!=1: raise RuntimeError('B06: suffix anchor missing')
   source=source.replace(anchor,'strings.Join(words[1:7], " ")',1)
  path.write_text(source)
  pattern='/'.join('^'+re.escape(part)+'$' for part in test.split('/'))
  command=['go','test',package,'-count=1','-v','-timeout=90s','-run',pattern]
  with (out/f'{ident}.log').open('w') as log:
   log.write('COMMAND: '+' '.join(command)+'\n');log.flush()
   result=subprocess.run(command,cwd=root,stdout=log,stderr=subprocess.STDOUT,timeout=110)
   log.write(f'\nEXIT: {result.returncode}\n')
  evidence=(out/f'{ident}.log').read_text(errors='replace')
  killed=(result.returncode==1 and f'--- FAIL: {test} ' in evidence and assertion in evidence
   and not any(s in evidence for s in ['[build failed]','test timed out','test command deadline','process survived','helper survived','manager survived']))
  rows.append(f'{ident}\t{test}\t{result.returncode}\t'+('killed' if killed else 'SURVIVOR/unverified'))
  failed |= not killed;print(rows[-1],flush=True)
 finally:
  path.write_bytes(original)
  if path.read_bytes()!=original: raise RuntimeError(f'{ident}: restore mismatch')
(out/'summary.tsv').write_text('\n'.join(rows)+'\n')
sys.exit(1 if failed else 0)
