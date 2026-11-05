package network

import (
	"errors"
	"io"
	"os"
	"path/filepath"
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

// policyCachePath returns only schema paths, never supplied values or unknown
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
	if r, ok := v.Get("restrictions"); !ok || r.Kind != fragment.KindObject {
		return "restrictions"
	}
	for _, m := range v.Obj {
		switch m.Key {
		case "restrictions":
			for _, r := range m.Value.Obj {
				path := "restrictions." + r.Key
				if r.Key != "allow_connector_file_downloads" && r.Key != "enforce_web_search_mcp_isolation" {
					return "restrictions.*"
				}
				allowed, ok := r.Value.Get("allowed")
				if r.Value.Kind != fragment.KindObject || len(r.Value.Obj) != 1 || !ok || allowed.Kind != fragment.KindBool {
					return path + ".allowed"
				}
				// 2.1.316 G5t consumes true as enforcement; false removes it.
				if r.Key == "enforce_web_search_mcp_isolation" && !allowed.Bool {
					return path + ".allowed"
				}
			}
		case "defaults":
			if m.Value.Kind != fragment.KindObject {
				return "defaults"
			}
			for _, d := range m.Value.Obj {
				if d.Key != "remote_control_at_startup" {
					return "defaults.*"
				}
				// 2.1.316 ors -> L8r uses true to enable startup remote control.
				if d.Value.Kind != fragment.KindBool || d.Value.Bool {
					return "defaults.remote_control_at_startup"
				}
			}
		case "compliance_taints":
			if m.Value.Kind != fragment.KindArray || len(m.Value.Arr) != 0 {
				return "compliance_taints"
			}
		case "monitoring_notice":
			// The native object schema catches a string to null; neither this
			// agreed string subset nor null changes settings or tool authority.
			if m.Value.Kind != fragment.KindNull && m.Value.Kind != fragment.KindString {
				return "monitoring_notice"
			}
		default:
			return "$.*"
		}
	}
	return ""
}

func checkPolicyCaches(clock guardClock, name, home string) error {
	refuse := func(file, path string) error {
		return refusal.New(refusal.CodeScopeUnsupported, name, "server policy cache "+file+" is not admitted at "+path)
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
			return refuse(file, "$")
		}
		if err := clock.step(name); err != nil {
			return err
		}
		if path := policyCachePath(doc, file == "remote-settings.json"); path != "" {
			return refuse(file, path)
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
			return refuse(file, "$")
		}
	}
	return clock.step(name)
}
