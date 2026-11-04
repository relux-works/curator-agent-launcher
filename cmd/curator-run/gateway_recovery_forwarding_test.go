//go:build darwin || linux

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// The explicitly selected, hash-checked real manager dispatches a fixture
// provider through its supported configuration. Its captured argv then drives
// the normal launcher path. No installed profile, home or service is changed;
// runtime and service endpoints belong only to these fixtures. This qualifies
// the selected manager bytes, not every version or a live member launch.
func TestGatewayRecoveryCanonicalForwarding(t *testing.T) {
	manager := os.Getenv("CURATOR_TEST_CANONICAL_MANAGER")
	if manager == "" {
		t.Skip("external qualification requires an explicitly selected manager and digest")
	}
	expected := os.Getenv("CURATOR_TEST_CANONICAL_MANAGER_SHA256")
	if !filepath.IsAbs(manager) || len(expected) != 64 {
		t.Fatal("explicit manager qualification input invalid")
	}
	raw, err := os.ReadFile(manager)
	if err != nil {
		t.Fatal("selected manager unavailable")
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != expected {
		t.Fatal("selected manager bytes differ from qualification input")
	}
	for _, variant := range []string{"selected", "legacy", "malformed", "native-tail"} {
		t.Run(variant, func(t *testing.T) {
			var f *pipelineFixture
			var recovery *launchRecoveryGateway
			var legacy *e2eGateway
			if variant == "selected" || variant == "malformed" {
				f, recovery = recoveryLaunchFixture(t)
			} else {
				f = printNetworkFixture(t)
				legacy = newE2EGateway(t)
				writeGatewayCatalog(t, f, legacy.socket)
			}
			providerDir := filepath.Join(f.dir, "capture-provider")
			captureDir := filepath.Join(f.dir, "capture-cwd")
			for _, p := range []string{providerDir, captureDir} {
				if err := os.Mkdir(p, 0700); err != nil {
					t.Fatal(err)
				}
			}
			helper, err := os.ReadFile(pipelineHelper)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(providerDir, "curator-run"), helper, 0700); err != nil {
				t.Fatal(err)
			}
			config, _ := json.Marshal(map[string]any{"schema_version": 2, "security_posture": "hardened", "skills_root": filepath.Join(f.dir, "skills"), "projects": map[string]any{}, "environments": map[string]any{"provider_directories": []string{providerDir}}})
			system := []byte(`{"schema_version":2}`)
			configPath, systemPath := filepath.Join(f.dir, "manager-user.json"), filepath.Join(f.dir, "manager-system.json")
			for path, data := range map[string][]byte{configPath: config, systemPath: system} {
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			args := []string{"claude", "--native", "--network", "egress-a", "--permissions", "native"}
			want := append([]string{"claude_code"}, args[1:]...)
			switch variant {
			case "selected":
				args = append(args, "--gateway-recovery")
				want = append(want, "--gateway-recovery")
			case "malformed":
				args = append(args, "--gateway-recovery=true")
				want = append(want, "--gateway-recovery=true")
			}
			args = append(args, "--", "-p", "hello")
			want = append(want, "--", "-p", "hello")
			if variant == "native-tail" {
				args = append(args, "--gateway-recovery")
				want = append(want, "--gateway-recovery")
			}
			cmd := exec.Command(manager, append([]string{"run"}, args...)...)
			cmd.Dir = captureDir
			// Keep the actual HOME: config isolation uses the documented selectors,
			// with no fake native home, inherited identity or credential environment.
			home, err := os.UserHomeDir()
			if err != nil {
				t.Fatal(err)
			}
			cmd.Env = []string{"HOME=" + home, "PATH=" + providerDir, "CURATOR_CONFIG=" + configPath, "CURATOR_SYSTEM_CONFIG=" + systemPath}
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("canonical fixture dispatch refused: %v; %s", err, stderr.Bytes())
			}
			var capture childCapture
			if json.Unmarshal(stdout.Bytes(), &capture) != nil || !reflect.DeepEqual(capture.Argv, want) {
				t.Fatal("CANONICAL FORWARDING ALARM: argument bytes changed before launcher")
			}
			for path, wantBytes := range map[string][]byte{configPath: config, systemPath: system} {
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(got, wantBytes) {
					t.Fatal("canonical dispatch rewrote fixture configuration")
				}
			}
			f.args = capture.Argv
			code, out, diagnostic := f.run()
			if variant == "native-tail" {
				// The manager preserves the native suffix; the gateway's existing
				// native-argument guard then refuses this unsupported option.
				if code != 1 || len(out) != 0 || legacy.calls() != "" || !strings.Contains(string(diagnostic), "network_configuration_conflict") {
					t.Fatal("forwarded native suffix bypassed its existing no-effect guard")
				}
				f.assertNoChild(t)
			} else if variant == "malformed" {
				if code != 2 || len(out) != 0 || recovery.calls() != "" {
					t.Fatal("forwarded malformed opt-in was not a no-effect usage refusal")
				}
				f.assertNoChild(t)
			} else {
				if code != 0 {
					t.Fatalf("forwarded valid launch refused: %d %s", code, diagnostic)
				}
				if recovery != nil {
					if !strings.Contains(recovery.calls(), "/v2/register") {
						t.Fatal("forwarded opt-in lost control2 selection")
					}
					recovery.retained(t, true)
				} else if !strings.Contains(legacy.calls(), "/v1/register") {
					t.Fatal("forwarded legacy input changed wire")
				}
			}
		})
	}
	final, err := os.ReadFile(manager)
	if err != nil || !bytes.Equal(raw, final) {
		t.Fatal("selected manager changed during qualification")
	}
}
