package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// The native, unmanaged Codex path carries an unshadowed caller selector to
// an inert child byte for byte and does not synthesize an absent selector.
// A later caller export replaces it, including with an empty value, without
// a selector warning. Injecting it into the closed Core fragment is
// refused before the builder and child; the nearby inherited case is valid.
// The scripted Core resolver proves neither real Core admission nor identity
// authentication, enrollment, or a reserved-variable preservation policy.
func TestInheritedPublicSelectorReachesInertChild(t *testing.T) {
	const name = "SWARMA_LAUNCH_IDENTITY"
	caller := "syn1:" + strings.Repeat("a", 64)
	replacement := "syn1:" + strings.Repeat("b", 64)
	for _, tc := range []struct {
		name    string
		seed    bool
		overlay string
		want    string
	}{
		{name: "absent"},
		{name: "inherited", seed: true, want: caller},
		{name: "caller-overlay", seed: true, overlay: "caller", want: replacement},
		{name: "empty-caller-overlay", seed: true, overlay: "caller"},
		{name: "fragment-injection-refused", seed: true, overlay: "fragment", want: replacement},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := entryFixture(t, "codex_cli")
			insertLauncherArgs(f, "--permissions", "native")
			if tc.seed {
				f.addEnv(name, caller)
			}
			if tc.overlay == "caller" {
				f.addEnv(name, tc.want)
			}
			if tc.overlay == "fragment" {
				var obj map[string]any
				if err := json.Unmarshal([]byte(f.resolver.stdout), &obj); err != nil {
					t.Fatal(err)
				}
				obj["env"].(map[string]any)[name] = tc.want
				raw, err := json.Marshal(obj)
				if err != nil {
					t.Fatal(err)
				}
				f.resolver.stdout = string(raw)
			}
			code, out, stderr := f.run()
			if tc.overlay == "fragment" {
				if code != 1 || f.builds != 0 || len(out) != 0 || !bytes.Contains(stderr, []byte("resolve_fragment_invalid:")) || !bytes.Contains(stderr, []byte("/env/"+name+": unknown member")) {
					t.Fatalf("fragment refusal: exit=%d builds=%d stderr=%q", code, f.builds, stderr)
				}
				f.assertNoChild(t)
				return
			}
			if code != 0 || f.builds != 1 {
				t.Fatalf("exit=%d builds=%d stderr=%q", code, f.builds, stderr)
			}
			var child childCapture
			if err := json.Unmarshal(out, &child); err != nil {
				t.Fatalf("inert child capture: %v", err)
			}
			var got string
			count := 0
			for _, entry := range child.Env {
				if value, ok := strings.CutPrefix(entry, name+"="); ok {
					got, count = value, count+1
				}
			}
			wantCount := 0
			if tc.seed || tc.overlay != "" {
				wantCount = 1
			}
			if count != wantCount || got != tc.want {
				t.Fatalf("selector inheritance: count=%d want=%d value=%q want=%q", count, wantCount, got, tc.want)
			}
			if tc.overlay == "caller" && got == caller {
				t.Fatal("overlay incorrectly preserved the caller selector")
			}
			// These successful fixtures have no selector diagnostic. Other
			// fixture diagnostics remain allowed; warning wording is not fixed.
			if bytes.Contains(stderr, []byte(name)) {
				t.Fatalf("unexpected selector diagnostic: %q", stderr)
			}
		})
	}
}
