#!/usr/bin/env bash
# Narrowing-mutant harness for the SPEC §4.1 fragment gates.
#
# Each mutant keeps the gate present and weakens it to admit exactly one
# member of the class it must reject (argv shape, exit mapping, stderr
# transport, CCJ-1 reader rules, schema closure, canonical emission, the
# entry-point wiring). For every mutant the whole behavioral suite runs
# (`go test ./...`), and the harness requires the named test to be among the
# failures. Sources are restored from byte copies, never via git.
#
# usage: .scripts/fragment-mutants.sh [evidence-dir]
set -u
cd "$(dirname "$0")/.."
out="${1:-.temp/fragment-mutants}"
mkdir -p "$out"

declare -A FILES=(
  [resolve]=internal/fragment/resolve.go
  [json]=internal/fragment/json.go
  [ccj1]=internal/fragment/ccj1.go
  [fragment]=internal/fragment/fragment.go
  [main]=cmd/curator-run/main.go
)
for k in "${!FILES[@]}"; do cp "${FILES[$k]}" "$out/$k.orig"; done
restore() { for k in "${!FILES[@]}"; do cp "$out/$k.orig" "${FILES[$k]}"; done; }
trap restore EXIT

# Fields separated by "@@": id, file key, expected failing test, perl -0pi
# expression (multi-line aware), description.
mutants=(
  'M01@@resolve@@TestNonZeroExitMapping@@s/"environment_repair_failed":    CodeRepairFailed,/"environment_repair_failed":    CodeRepairFailed, "environment_home_stale": CodeRepairFailed,/@@map the unreachable environment_home_stale to resolve_repair_failed'
  'M02@@resolve@@TestArgvExactOrderAndRepair@@s/return append\(argv, "--repair", "--format", "json"\)/return append(argv, "--format", "json")/@@drop --repair from argv'
  'M03@@resolve@@TestArgvExactOrderAndRepair@@s/argv = append\(argv, "--profile", r.Profile\)/argv = append(argv, "--profile="+r.Profile)/@@pass --profile=<name> as one token'
  'M04@@resolve@@TestNonZeroExitNeverYieldsFragmentEvenWithValidStdout@@s/if exit != 0 \{/if exit != 0 \&\& exit != 1 {/@@treat exit 1 with parseable stdout as success'
  'M05@@resolve@@TestInvalidOutputIsFragmentInvalidNeverAbsence@@s/f, err := Parse\(stdout\)\n\tif err != nil \{/f, err := Parse(stdout)\n\tif err != nil \&\& len(bytes.TrimSpace(stdout)) != 0 {/@@empty stdout treated as absence'
  'M06@@resolve@@TestInvalidOutputIsFragmentInvalidNeverAbsence@@s/if f.Environment != req.EnvID \{/if f.Environment != req.EnvID \&\& f.Environment != "codex_cli" {/@@accept a codex_cli fragment for any requested env'
  'M07@@resolve@@TestNonZeroExitMapping@@s/rest, ok := strings.CutPrefix\(line, "curator: "\)\n\t\tif !ok \{\n\t\t\tcontinue\n\t\t\}/idx := strings.Index(line, "curator: ")\n\t\tif idx < 0 {\n\t\t\tcontinue\n\t\t}\n\t\trest := line[idx+len("curator: "):]/@@token preserved: match "curator: " anywhere in the line'
  'M08@@resolve@@TestNonZeroExitMapping@@s/stdout, exit, startErr := r.runner.Run\(ctx, r.binary, req.Argv\(\), req.Dir, env, io.MultiWriter\(stderr, &captured\)\)/stdout, exit, startErr := r.runner.Run(ctx, r.binary, req.Argv(), req.Dir, env, io.MultiWriter(stderr, \&captured))\n\tif exit == 1 {\n\t\tstdout, exit, startErr = r.runner.Run(ctx, r.binary, req.Argv(), req.Dir, env, io.MultiWriter(stderr, \&captured))\n\t}/@@retry once on exit 1'
  'M09@@resolve@@TestResolveSuccessForwardsWarnings@@s/io.MultiWriter\(stderr, &captured\)\)\n\tif startErr/\&captured)\n\tif exit != 0 {\n\t\tstderr.Write(captured.Bytes())\n\t}\n\tif startErr/@@forward stderr only on failure'
  'M10@@json@@TestRejections@@s/if seen\[key\] \{/if seen[key] \&\& key != "environment" {/@@duplicate "environment" key admitted'
  'M11@@json@@TestReaderRejections@@s/case r >= 0xDC00 && r <= 0xDFFF:\n\t\t\t\t\treturn "", p.errf\("lone low surrogate escape"\)/case r >= 0xDC00 \&\& r <= 0xDFFF:\n\t\t\t\t\tr = 0xFFFD/@@lone low surrogate repaired instead of rejected'
  'M12@@json@@TestReaderRejections@@s/case \x27.\x27, \x27e\x27, \x27E\x27:\n\t\t\treturn Value\{\}, p.errf\("non-integer number"\)/case \x27.\x27, \x27E\x27:\n\t\t\treturn Value{}, p.errf("non-integer number")\n\t\tcase \x27e\x27:\n\t\t\tp.pos++\n\t\t\tfor p.pos < len(p.data) \&\& p.data[p.pos] >= \x270\x27 \&\& p.data[p.pos] <= \x279\x27 {\n\t\t\t\tp.pos++\n\t\t\t}/@@admit lowercase exponent numbers'
  'M13@@json@@TestReaderRejections@@s/if p.pos != len\(p.data\) \{\n\t\treturn Value\{\}, p.errf\("trailing content after the JSON value"\)/if p.pos != len(p.data) \&\& p.data[p.pos] != \x27{\x27 {\n\t\treturn Value{}, p.errf("trailing content after the JSON value")/@@trailing second object admitted'
  'M14@@ccj1@@TestA0FragmentsParseAndDigestMatch@@s/\t\tcase \x27\\t\x27:\n\t\t\tb = append\(b, \x27\\\\\x27, \x27t\x27\)/\t\tcase \x27\\t\x27:\n\t\t\tb = append(b, \x27\\\\\x27, \x27t\x27)\n\t\tcase \x27\/\x27:\n\t\t\tb = append(b, \x27\\\\\x27, \x27\/\x27)/@@escape "\/" in canonical strings'
  'M15@@ccj1@@TestCanonicalRules@@s/\t\t\t\tb = append\(b, c\)\n\t\t\t\}/\t\t\t\tif c == 0xE2 \&\& i+2 < len(s) \&\& s[i+1] == 0x80 \&\& (s[i+2] == 0xA8 || s[i+2] == 0xA9) {\n\t\t\t\t\tb = append(b, \x27\\\\\x27, \x27u\x27, \x272\x27, \x270\x27, \x272\x27, hexDigits[s[i+2]-0xA0])\n\t\t\t\t\ti += 2\n\t\t\t\t} else {\n\t\t\t\t\tb = append(b, c)\n\t\t\t\t}\n\t\t\t}/@@escape U+2028 and U+2029'
  'M16@@ccj1@@TestCanonicalRules@@s/sort.Strings\(keys\)/sort.Slice(keys, func(i, j int) bool { return utf16Less(keys[i], keys[j]) })/; s/const hexDigits/func utf16Less(a, b string) bool {\n\tx, y := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))\n\tfor i := 0; i < len(x) \&\& i < len(y); i++ {\n\t\tif x[i] != y[i] {\n\t\t\treturn x[i] < y[i]\n\t\t}\n\t}\n\treturn len(x) < len(y)\n}\n\nconst hexDigits/; s/"strconv"\n\)/"strconv"\n\t"unicode\/utf16"\n)/@@sort keys by UTF-16 code units'
  'M17@@fragment@@TestConformanceCorpus@@s/if m.Key == "composition" \{\n\t\t\t\treturn invalid\(path\+"\/"\+m.Key, "member withdrawn under Decision 0012 D8"\)\n\t\t\t\}/if m.Key == "composition" {\n\t\t\t\tcontinue\n\t\t\t}/@@accept the withdrawn composition member'
  'M18@@fragment@@TestConformanceCorpus@@s/EnvOpenCode:   \{Kind: KindVariable, Variable: "OPENCODE_CONFIG"\},\n\}/EnvOpenCode:   {Kind: KindVariable, Variable: "OPENCODE_CONFIG"},\n\tEnvPi:         {Kind: KindFlag, Flag: "--mcp-config", Argument: ArgumentPath, With: []string{"--strict-mcp-config"}},\n}/@@registry grants pi an MCP descriptor'
  'M19@@fragment@@TestConformanceCorpus@@s/string\(SemanticsAppend\), string\(SemanticsReplace\)\)\n\t\tif err != nil \{\n\t\t\treturn Channel\{\}, err/string(SemanticsAppend), string(SemanticsReplace), "prepend")\n\t\tif err != nil {\n\t\t\treturn Channel{}, err/; s/x.Kind != y.Kind \|\| x.Semantics != y.Semantics \|\|/x.Kind != y.Kind || (x.Semantics != y.Semantics \&\& x.Semantics != "prepend") ||/@@admit semantics prepend at both layers'
  'M20@@fragment@@TestInvalidErrorShape@@s/if len\(with.Arr\) == 0 \{/if len(with.Arr) == 0 \&\& false {/@@admit an empty with list (registry compare still rejects; the path-level test catches it)'
  'M21@@fragment@@TestRejections@@s/if i > 0 && n.Str <= mc.EnvNames\[i-1\] \{/if i > 0 \&\& n.Str < mc.EnvNames[i-1] {/@@admit duplicate env_names'
  'M22@@fragment@@TestRejections@@s/if !sameChannels\(s.Channels, systemPromptRegistry\[env\]\) \{/if len(s.Channels) != 0 \&\& !sameChannels(s.Channels, systemPromptRegistry[env]) \&\& len(s.Channels) != 1 {/@@admit a one-descriptor subset of the system-prompt list'
  'M23@@fragment@@TestConformanceCorpus@@s/if rootDir == "" \|\| !strings.HasPrefix\(pp.Str, rootDir\+"\/"\) \{/if rootDir == "" || !(strings.HasPrefix(pp.Str, rootDir+"\/") || pp.Str == "\/usr\/local\/bin") {/@@admit /usr/local/bin as path_prepend'
  'M24@@fragment@@TestConformanceCorpus@@s/if len\(envObj.Obj\) != 1 \{/if len(envObj.Obj) > 2 || len(envObj.Obj) == 0 {/@@admit two env variables'
  'M25@@fragment@@TestConformanceCorpus@@s/if !hex256Pattern.MatchString\(f.Profile.LockSHA256\) \{/if !hex256Pattern.MatchString(strings.TrimPrefix(f.Profile.LockSHA256, "sha256:")) {/@@admit a sha256:-prefixed lock hash'
  'M26@@main@@TestRunResolveFailuresExit1@@s/if re, ok := fragment.IsResolve\(err\); ok \{/if re, ok := fragment.IsResolve(err); ok \&\& re.Code != fragment.CodeFragmentInvalid {/@@entry point loses the resolve_fragment_invalid code line'
)

status=0
summary="$out/summary.tsv"
printf 'mutant\tfile\texpected_failing_test\tapplied\tsuite_exit\texpected_test_failed\tverdict\tdescription\n' >"$summary"
for m in "${mutants[@]}"; do
  id="${m%%@@*}"; rest="${m#*@@}"
  which="${rest%%@@*}"; rest="${rest#*@@}"
  test="${rest%%@@*}"; rest="${rest#*@@}"
  expr="${rest%%@@*}"; desc="${rest#*@@}"
  restore
  file="${FILES[$which]}"
  perl -0pi -e "$expr" "$file"
  if cmp -s "$file" "$out/$which.orig"; then
    printf '%s\t%s\t%s\tno\t-\t-\tNOT_APPLIED\t%s\n' "$id" "$which" "$test" "$desc" >>"$summary"; status=1; continue
  fi
  log="$out/$id.log"
  { echo "== $id ($desc; expected to fail: $test)"; diff -u "$out/$which.orig" "$file"; echo; } >"$log"
  go test ./... -count=1 -v >>"$log" 2>&1; code=$?
  failed=no
  grep -Eq "^[[:space:]]*--- FAIL: ${test}( |$)" "$log" && failed=yes
  verdict=KILLED
  if [ "$code" -eq 0 ] || [ "$failed" = no ]; then verdict=SURVIVED; status=1; fi
  printf '%s\t%s\t%s\tyes\t%s\t%s\t%s\t%s\n' "$id" "$which" "$test" "$code" "$failed" "$verdict" "$desc" >>"$summary"
done
restore
column -t -s $'\t' "$summary"
exit "$status"
