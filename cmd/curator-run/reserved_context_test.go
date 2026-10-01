package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

// Enumerate the reserved tags from the fragment package's definitions, rather
// than from the projection being tested. New reserved fields need a valid input
// fixture here, and must also remain absent from the execution carrier.
func TestProductionContextCarrierReservedMembersPreservePathAndDigest(t *testing.T) {
	members := map[string]string{}
	typ := reflect.TypeFor[fragment.Fragment]()
	for i := range typ.NumField() {
		field := typ.Field(i)
		if wire, ok := field.Tag.Lookup("reserved"); ok {
			if wire == "" || field.Type.Kind() != reflect.String {
				t.Fatalf("reserved field %s needs a valid nonempty fixture", field.Name)
			}
			members[field.Name] = wire
		}
	}
	if members["PathPrepend"] != "path_prepend" {
		t.Fatal("fragment definition must mark path_prepend reserved")
	}
	for _, environment := range []string{"claude_code", "codex_cli"} {
		for _, tracked := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", environment, tracked), func(t *testing.T) {
				f := entryFixture(t, environment, tracked)
				baseline, err := fragment.Parse([]byte(f.resolver.stdout))
				if err != nil {
					t.Fatal(err)
				}
				var obj map[string]any
				if err := json.Unmarshal([]byte(f.resolver.stdout), &obj); err != nil {
					t.Fatal(err)
				}
				for _, wire := range members {
					obj[wire] = f.home + "/reserved-bin"
				}
				raw, err := json.Marshal(obj)
				if err != nil {
					t.Fatal(err)
				}
				parsed, err := fragment.Parse(raw)
				if err != nil {
					t.Fatalf("reserved members must remain schema-valid: %v", err)
				}
				for field, wire := range members {
					if reflect.ValueOf(*parsed).FieldByName(field).String() != obj[wire] {
						t.Fatalf("reserved member %s was not parsed", wire)
					}
				}
				sum := sha256.Sum256(parsed.Canonical)
				wantDigest := fragment.DigestPrefix + hex.EncodeToString(sum[:])
				if parsed.Digest != wantDigest || parsed.Digest == baseline.Digest {
					t.Fatal("reserved members must remain included in the fragment digest")
				}
				f.resolver.stdout = string(raw)
				code, out, stderr := f.run()
				if code != 0 {
					t.Fatalf("reserved members must preserve launch: exit=%d stderr=%s", code, stderr)
				}
				if f.builds != 1 || f.verdicts != 1 || f.request.Context == nil {
					t.Fatal("launch must use the real carrier admission path")
				}
				carrier, err := json.Marshal(f.request.Context)
				if err != nil {
					t.Fatal(err)
				}
				var channels map[string]any
				if err := json.Unmarshal(carrier, &channels); err != nil {
					t.Fatal(err)
				}
				for field, wire := range members {
					value := reflect.ValueOf(*f.request.Context).FieldByName(field)
					if value.IsValid() && !value.IsZero() {
						t.Fatalf("reserved member %s reached the carrier", wire)
					}
					if _, exists := channels[wire]; exists {
						t.Fatalf("reserved member %s reached the carrier wire representation", wire)
					}
				}
				if !slices.Contains(f.plan.Env, "PATH="+f.dir) {
					t.Fatalf("reserved members changed admitted PATH: %q", f.plan.Env)
				}
				_, env, names := capturedLaunch(t, out, tracked)
				if tracked {
					if _, exists := env["PATH"]; exists || slices.Contains(names, "PATH") {
						t.Fatal("reserved members introduced a tracked PATH override")
					}
					var child childCapture
					if err := json.Unmarshal(out, &child); err != nil {
						t.Fatal(err)
					}
					var doc struct {
						Extensions map[string]any `json:"extensions"`
					}
					if err := json.Unmarshal(child.Stdin, &doc); err != nil {
						t.Fatal(err)
					}
					if doc.Extensions["works.relux.curator.fragment-digest"] != wantDigest {
						t.Fatal("tracked transport changed the original fragment digest")
					}
				} else if env["PATH"] != f.dir {
					t.Fatalf("reserved members changed direct PATH: %q", env["PATH"])
				}
				if f.resolver.stdout != string(raw) || strings.Contains(string(carrier), "/reserved-bin") {
					t.Fatal("projection changed the original fragment or leaked reserved data")
				}
			})
		}
	}
}
