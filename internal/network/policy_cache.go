package network

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-network-profiles/pkg/refusal"
)

const maxPolicySidecarBytes int64 = 64 << 10

// Cache files have a stricter contract than ordinary settings: never follow
// a final link, including one installed between inspection and open.
var openPolicyCache = func(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
}

func readPolicyCache(clock guardClock, name, path string) ([]byte, error) {
	step := func() error {
		if err := clock.step(name); err != nil {
			return clockStop{err}
		}
		return nil
	}
	if err := step(); err != nil {
		return nil, err
	}
	before, err := inspectPath(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errAbsent
	}
	if err != nil || !before.Mode().IsRegular() || before.Size() > maxSettingsBytes || before.Mode().Perm()&0o444 == 0 {
		return nil, errors.New("not a bounded readable regular file")
	}
	if err := step(); err != nil {
		return nil, err
	}
	f, err := openPolicyCache(path)
	if err != nil {
		return nil, errors.New("cannot open cache without following links")
	}
	defer closeSettingsFile(f)
	if err := step(); err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) || opened.Size() != before.Size() || !opened.ModTime().Equal(before.ModTime()) {
		return nil, errors.New("cache changed during open")
	}
	var doc []byte
	buf := make([]byte, settingsChunk)
	for {
		if err := step(); err != nil {
			return nil, err
		}
		n, err := readSettingsChunk(f, buf)
		doc = append(doc, buf[:n]...)
		if len(doc) > maxSettingsBytes {
			return nil, errors.New("cache exceeds limit")
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errors.New("cannot read cache")
		}
	}
	if err := step(); err != nil {
		return nil, err
	}
	after, err := f.Stat()
	if err != nil || after.Size() != opened.Size() || int64(len(doc)) != opened.Size() || !after.ModTime().Equal(opened.ModTime()) {
		return nil, errors.New("cache changed during read")
	}
	if err := step(); err != nil {
		return nil, err
	}
	current, err := inspectPath(path)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(after, current) || current.Size() != after.Size() || !current.ModTime().Equal(after.ModTime()) {
		return nil, errors.New("cache replaced during read")
	}
	return doc, nil
}

// policyCachePath returns all rejected schema paths, never supplied values or unknown
// member names (which themselves could contain sensitive text). The strict
// parser rejects escaped duplicate keys, invalid Unicode and trailing input.
func policyCachePath(doc []byte, remote bool) string {
	v, err := fragment.ParseJSON(doc)
	if err != nil || v.Kind != fragment.KindObject {
		return "$"
	}
	if remote {
		if len(v.Obj) != 0 {
			return "$.*"
		}
		return ""
	}
	paths := map[string]bool{}
	add := func(path string) { paths[path] = true }
	if r, ok := v.Get("restrictions"); !ok || r.Kind != fragment.KindObject {
		add("restrictions")
	}
	for _, m := range v.Obj {
		switch m.Key {
		case "restrictions":
			if m.Value.Kind != fragment.KindObject {
				continue
			}
			for _, r := range m.Value.Obj {
				path := "restrictions." + r.Key
				if r.Key != "allow_connector_file_downloads" && r.Key != "enforce_web_search_mcp_isolation" {
					add("restrictions.*")
					continue
				}
				allowed, ok := r.Value.Get("allowed")
				if r.Value.Kind != fragment.KindObject || !ok || allowed.Kind != fragment.KindBool {
					add(path + ".allowed")
				}
				for _, member := range r.Value.Obj {
					if member.Key != "allowed" {
						add(path + ".*")
					}
				}
				// Either bool is inert under the existing gateway input boundary:
				// the pinned loopback base and refused socket/first-party/client-data
				// overrides make native policy ineligible in 2.1.316 and 2.1.318.
				// Outside that boundary false can suppress hinted enforcement.
			}
		case "defaults":
			if m.Value.Kind != fragment.KindObject {
				add("defaults")
				continue
			}
			for _, d := range m.Value.Obj {
				if d.Key != "remote_control_at_startup" {
					add("defaults.*")
					continue
				}
				// 2.1.316 ors -> L8r uses true to enable startup remote control.
				if d.Value.Kind != fragment.KindBool || d.Value.Bool {
					add("defaults.remote_control_at_startup")
				}
			}
		case "compliance_taints":
			if m.Value.Kind != fragment.KindArray || len(m.Value.Arr) != 0 {
				add("compliance_taints")
			}
		case "monitoring_notice":
			// The native object schema catches a string to null; neither this
			// agreed string subset nor null changes settings or tool authority.
			if m.Value.Kind != fragment.KindNull && m.Value.Kind != fragment.KindString {
				add("monitoring_notice")
			}
		default:
			add("$.*")
		}
	}
	var ordered []string
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	return strings.Join(ordered, ", ")
}

func checkPolicyCaches(clock guardClock, name, home string) error {
	var rejected []string
	refuse := func(file, path string) {
		rejected = append(rejected, file+" at "+path)
	}
	for _, file := range []string{"remote-settings.json", "policy-limits.json"} {
		doc, err := readPolicyCache(clock, name, filepath.Join(home, file))
		var stop clockStop
		if errors.As(err, &stop) {
			return stop.error
		}
		if errors.Is(err, errAbsent) {
			continue
		}
		if err != nil {
			refuse(file, "$")
			continue
		}
		if err := clock.step(name); err != nil {
			return err
		}
		if path := policyCachePath(doc, file == "remote-settings.json"); path != "" {
			refuse(file, path)
		}
	}
	// These companions carry identity/signature/age metadata, not an admission
	// grant. Inspect metadata only; native contents are never read or trusted.
	for _, file := range []string{
		"policy-limits.json.stamp.json", "policy-limits.json.signature.json", "policy-limits.json.signature-iat.json",
		"remote-settings.json.signature.json", "remote-settings.json.signature-iat.json", "remote-settings.json.synced-at",
	} {
		if err := clock.step(name); err != nil {
			return err
		}
		info, err := inspectPath(filepath.Join(home, file))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxPolicySidecarBytes {
			refuse(file, "$")
		}
	}
	if err := clock.step(name); err != nil {
		return err
	}
	if len(rejected) != 0 {
		return refusal.New(refusal.CodeScopeUnsupported, name, "server policy caches are not admitted: "+strings.Join(rejected, "; "))
	}
	return nil
}
