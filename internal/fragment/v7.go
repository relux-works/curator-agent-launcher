package fragment

import (
	"path"
	"strings"
)

const (
	// AntigravityForceFileTokenVariable is the launch-env constant required on
	// every managed Antigravity launch (CIP-0033).
	AntigravityForceFileTokenVariable = "AGY_CLI_FORCE_FILE_TOKEN_STORAGE"
	// AntigravityForceFileTokenTrue is the sole valid value for the forced
	// file-token storage variable on managed Antigravity launches.
	AntigravityForceFileTokenTrue = "true"
	// AntigravityMCPFile is the relative MCP config path under the managed home.
	AntigravityMCPFile = "config/mcp_config.json"
	// AntigravityDisableAutoUpdateVariable is pinned by SAM ChildEnv on managed launches.
	AntigravityDisableAutoUpdateVariable = "AGY_CLI_DISABLE_AUTO_UPDATE"
)

// IsAntigravityForbiddenEnv reports whether name is an ambient Antigravity
// authentication or managed-home policy variable that MUST NOT pass through
// MCP env_names, prompt overlays, plan-owned env overrides, or network patches
// (CIP-0033).
func IsAntigravityForbiddenEnv(name string) bool {
	switch strings.ToUpper(name) {
	case "GEMINI_API_KEY",
		"GOOGLE_API_KEY",
		"AGY_ADC_AUTH",
		"GOOGLE_APPLICATION_CREDENTIALS",
		"GOOGLE_CLOUD_QUOTA_PROJECT",
		AntigravityForceFileTokenVariable,
		AntigravityDisableAutoUpdateVariable:
		return true
	default:
		return false
	}
}

// v7HomePolicy validates the v7 `home` transport object. Antigravity requires
// {"path": <clean abs path, basename != ".gemini">, "transport": "flag"};
// env-home environments forbid `home`.
func v7HomePolicy(root Value, f *Fragment) error {
	hv, present := root.Get("home")
	if f.Environment != EnvAntigravity {
		if present {
			return invalid("/home", "home member is only admitted for flag-home environments")
		}
		return nil
	}
	if !present {
		return invalid("/home", "required for antigravity")
	}
	if err := closedObject("/home", hv, []string{"path", "transport"}, nil); err != nil {
		return err
	}
	homePath, err := stringMember("/home", hv, "path")
	if err != nil {
		return err
	}
	if err := checkAbsolutePath("/home/path", homePath); err != nil {
		return err
	}
	if path.Clean(homePath) != homePath {
		return invalid("/home/path", "%q must be a clean absolute POSIX path", homePath)
	}
	if path.Base(homePath) == ".gemini" {
		return invalid("/home/path", "managed antigravity home basename must not be .gemini")
	}
	transport, err := enumMember("/home", hv, "transport", "flag")
	if err != nil {
		return err
	}
	f.HomeBinding = &HomeBinding{Path: homePath, Transport: transport}
	return nil
}

// v7Policy validates v7 typed policy fields (`root_context` and
// `credential_store`). Non-Antigravity environments reuse v4Policy verbatim;
// Antigravity forbids `root_context` and requires `credential_store: {"selection":"file"}`.
func v7Policy(root Value, f *Fragment) error {
	if f.Environment != EnvAntigravity {
		return v4Policy(root, f)
	}
	if _, ok := root.Get("root_context"); ok {
		return invalid("/root_context", "antigravity does not admit root_context")
	}
	value, present := root.Get("credential_store")
	if !present {
		return invalid("/credential_store", "required for antigravity")
	}
	if err := closedObject("/credential_store", value, []string{"selection"}, []string{"config_overrides"}); err != nil {
		return err
	}
	selection, err := enumMember("/credential_store", value, "selection", "file")
	if err != nil {
		return err
	}
	if _, hasOverrides := value.Get("config_overrides"); hasOverrides {
		return invalid("/credential_store/config_overrides", "overrides forbidden for antigravity")
	}
	f.CredentialStore = &CredentialStore{Selection: selection}
	return nil
}

func checkPortableRelativePath(pathStr, s string) error {
	if s == "" || len(s) > 4096 || strings.HasPrefix(s, "/") || strings.HasSuffix(s, "/") {
		return invalid(pathStr, "%q is not a portable path", s)
	}
	for _, seg := range strings.Split(s, "/") {
		if err := checkPortableFilename(pathStr, seg); err != nil {
			return err
		}
	}
	return nil
}
