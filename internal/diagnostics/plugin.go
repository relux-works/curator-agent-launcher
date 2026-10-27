package diagnostics

import "sort"

// PluginDiagnostic defines launcher-owned text and registry exits for the
// front-door extension. Plugin-supplied messages never enter this table.
type PluginDiagnostic struct {
	Exit    int
	Message string
}

var pluginDiagnostics = map[string]PluginDiagnostic{
	"ax_tracking_removed":                      {2, "Ax tracking was removed; delete this file."},
	"credential_binding_mismatch":              {2, "Credential binding mismatch."},
	"credential_injection_unavailable":         {2, "Credential injection unavailable."},
	"credential_node_missing":                  {2, "Credential node missing."},
	"credential_reserved_name":                 {2, "Credential reserved name."},
	"credential_source_conflict":               {2, "Credential source conflict."},
	"credential_source_expired":                {2, "Credential source expired."},
	"credential_source_revoked":                {2, "Credential source revoked."},
	"credential_source_stale":                  {2, "Credential source stale."},
	"credential_store_unreadable":              {2, "Credential store unreadable."},
	"doorbell_setup_failed":                    {1, "Doorbell setup failed."},
	"environment_credential_store_unsupported": {2, "Environment credential store unsupported."},
	"fragment_version_unsupported":             {2, "Fragment version unsupported."},
	"session_board_goal_unavailable":           {1, "Session board goal unavailable."},
	"session_curator_identity_stale":           {1, "Session Curator identity is stale."},
	"session_generation_mismatch":              {1, "Session generation mismatch."},
	"session_host_capability_missing":          {6, "Required host capability is missing."},
	"session_host_delegated_unauthorized":      {16, "Delegated session hosting is not authorised."},
	"session_host_protocol_error":              {6, "Invalid session host response."},
	"session_liveness_unknown":                 {1, "Session liveness is unknown."},
	"session_operation_conflict":               {1, "Session operation conflict."},
	"session_operation_expired":                {1, "Session operation expired."},
	"session_resume_ambiguous":                 {1, "Session resume refused."},
	"session_resume_identity_mismatch":         {1, "Session resume refused."},
	"session_resume_location_mismatch":         {1, "Session resume refused."},
	"session_resume_not_found":                 {1, "Session resume refused."},
	"session_resume_unreadable":                {1, "Session resume refused."},
	"werkstatt_already_up":                     {1, "Werkstatt already up."},
	"werkstatt_busy":                           {1, "Werkstatt busy."},
	"werkstatt_clone_missing":                  {1, "Werkstatt clone missing."},
	"werkstatt_context_check_failed":           {2, "Werkstatt context check failed."},
	"werkstatt_context_leak":                   {2, "Werkstatt context leak."},
	"werkstatt_git_layout_invalid":             {2, "Werkstatt git layout invalid."},
	"werkstatt_identity_missing":               {1, "Werkstatt identity missing."},
	"werkstatt_liveness_write_failed":          {1, "Werkstatt liveness write failed."},
	"werkstatt_not_up":                         {1, "Werkstatt has no liveness record."},
	"werkstatt_not_found":                      {1, "Werkstatt not found."},
	"werkstatt_operation_unresolved":           {1, "Werkstatt operation unresolved."},
	"werkstatt_profile_changed":                {1, "Werkstatt profile changed."},
	"werkstatt_profile_lock_mismatch":          {1, "Werkstatt profile lock mismatch."},
	"werkstatt_record_invalid":                 {2, "Werkstatt record invalid."},
	"werkstatt_session_manager_required":       {1, "Werkstatt session manager required."},
}

func PluginDiagnosticFor(code string) (PluginDiagnostic, bool) {
	row, ok := pluginDiagnostics[code]
	return row, ok
}
func pluginCodes() []string {
	out := make([]string, 0, len(pluginDiagnostics))
	for code := range pluginDiagnostics {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}
