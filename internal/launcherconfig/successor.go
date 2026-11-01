package launcherconfig

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

// Proxy selects a profile from the existing network catalog, not a second
// endpoint registry. Execution must enforce the selected profile's contract.
type Proxy struct {
	Profile string `json:"profile"`
}

// ClaudeAuth contains store-selection metadata only. Parsing never
// resolve the source or enable a native/hosted execution route.
type ClaudeAuth struct {
	Type  string    `json:"type"`
	Store AuthStore `json:"store"`
}

// AuthStore is a closed union: file uses Path; keychain uses Service/Account.
// It cannot contain a token or a node-source enrollment projection.
type AuthStore struct {
	Type    string `json:"type"`
	Path    string `json:"path,omitempty"`
	Service string `json:"service,omitempty"`
	Account string `json:"account,omitempty"`
}

func reference(s string) bool {
	return s != "" && len(s) <= 4096 && utf8.ValidString(s) && strings.TrimSpace(s) == s &&
		!strings.ContainsFunc(s, unicode.IsControl)
}

func exactMembers(v fragment.Value, names ...string) bool {
	if v.Kind != fragment.KindObject || len(v.Obj) != len(names) {
		return false
	}
	for _, name := range names {
		if _, ok := v.Get(name); !ok {
			return false
		}
	}
	return true
}

func stringMember(v fragment.Value, key string) string {
	x, ok := v.Get(key)
	if !ok || x.Kind != fragment.KindString {
		return ""
	}
	return x.Str
}

func parseProxy(v fragment.Value) (*Proxy, error) {
	profile := stringMember(v, "profile")
	if !exactMembers(v, "profile") || !reference(profile) {
		return nil, fmt.Errorf("invalid proxy profile reference")
	}
	return &Proxy{Profile: profile}, nil
}

func parseClaudeAuth(v fragment.Value) (*ClaudeAuth, error) {
	fail := func() (*ClaudeAuth, error) { return nil, fmt.Errorf("invalid Claude OAuth store selection") }
	if !exactMembers(v, "type", "store") || stringMember(v, "type") != "oauth" {
		return fail()
	}
	store, _ := v.Get("store")
	selected := AuthStore{Type: stringMember(store, "type")}
	switch selected.Type {
	case "file":
		selected.Path = stringMember(store, "path")
		if !exactMembers(store, "type", "path") || !reference(selected.Path) ||
			!filepath.IsAbs(selected.Path) || filepath.Clean(selected.Path) != selected.Path {
			return fail()
		}
	case "keychain":
		selected.Service, selected.Account = stringMember(store, "service"), stringMember(store, "account")
		if !exactMembers(store, "type", "service", "account") || !reference(selected.Service) || !reference(selected.Account) {
			return fail()
		}
	default:
		return fail()
	}
	return &ClaudeAuth{Type: "oauth", Store: selected}, nil
}

// Marshal emits host policy only; launcher selections belong to Core.
func Marshal(c Config) ([]byte, error) {
	var mode *string
	if c.HostMode.Source == "machine" {
		value := c.HostMode.Value
		mode = &value
	} else if c.HostMode != (Mode{}) && c.HostMode != (Mode{"auto", "default"}) {
		return nil, fmt.Errorf("invalid host mode provenance")
	}
	var trial *bool
	if c.DelegatedTrialSet {
		value := c.DelegatedTrial
		trial = &value
	} else if c.DelegatedTrial {
		return nil, fmt.Errorf("delegated trial requires explicit selection")
	}
	data, err := json.Marshal(struct {
		Schema         string  `json:"schema"`
		HostMode       *string `json:"host_mode,omitempty"`
		DelegatedTrial *bool   `json:"delegated_trial,omitempty"`
	}{Schema, mode, trial})
	if err != nil {
		return nil, fmt.Errorf("invalid launcher configuration encoding")
	}
	if _, err = Parse(data); err != nil {
		return nil, err
	}
	return data, nil
}

// ValidateClaudeAuth checks a typed selection with the same closed parser as
// the Core projection. It reads neither the file nor the Keychain backend.
func ValidateClaudeAuth(selected ClaudeAuth) error {
	data, err := json.Marshal(selected)
	if err != nil {
		return fmt.Errorf("invalid Claude OAuth store selection")
	}
	value, err := fragment.ParseJSON(data)
	if err != nil {
		return fmt.Errorf("invalid Claude OAuth store selection")
	}
	_, err = parseClaudeAuth(value)
	return err
}
