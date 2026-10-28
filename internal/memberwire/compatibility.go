package memberwire

import (
	"strings"
	"unicode/utf8"
)

const (
	OwnerLimit   = 4 * 1024
	CatalogLimit = 64 * 1024
	TupleLimit   = 64 * 1024
)

// DecodeOwner checks only the closed owner shape. UID and identity fields are
// untrusted data, not observations of the caller, a store or authorization.
func DecodeOwner(raw []byte) (*UntrustedRecord, *Error) {
	return decode(raw, OwnerLimit, ownerValid)
}

// DecodeCatalog checks structure only, including empty catalogs and repeated
// tuple references. It does not admit entries or verify owner publications.
func DecodeCatalog(raw []byte) (*UntrustedRecord, *Error) {
	return decode(raw, CatalogLimit, catalogValid)
}

// DecodeTuple checks the complete published tuple shape, not compatibility,
// private provenance, effective configuration or permission to execute it.
func DecodeTuple(raw []byte) (*UntrustedRecord, *Error) {
	return decode(raw, TupleLimit, tupleValid)
}

// The strict parser has already checked UTF-8 and the global decoded-byte cap.
// Schema minLength/maxLength count Unicode scalar values, not UTF-8 bytes.
func textLength(v any, maximum int) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	n := utf8.RuneCountInString(s)
	return n >= 1 && n <= maximum
}

func revision(v any) bool { return lowerHex(v, 40) || lowerHex(v, 64) }
func repository(v any) bool {
	s, ok := v.(string)
	if !ok || len(s) > 2048 || !strings.HasPrefix(s, "https://") || len(s) == len("https://") {
		return false
	}
	for _, c := range s[len("https://"):] {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '/' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func componentValid(v any) bool {
	o := object(v)
	return closed(o, "repository", "revision", "binary_sha256", "release") && repository(o["repository"]) && revision(o["revision"]) && digest(o["binary_sha256"]) && textLength(o["release"], 128)
}
func ownerInputValid(v any) bool {
	o := object(v)
	return closed(o, "owner", "repository", "revision", "document") && is(o["owner"], "curator", "runtime", "launcher") && repository(o["repository"]) && revision(o["revision"]) && reference(o["document"])
}
func ownerInputsValid(v any, requireEachOwner bool) bool {
	a, ok := v.([]any)
	if !ok || len(a) < 3 || len(a) > 16 {
		return false
	}
	var curator, runtime, launcher bool
	for _, item := range a {
		if !ownerInputValid(item) {
			return false
		}
		switch object(item)["owner"] {
		case "curator":
			curator = true
		case "runtime":
			runtime = true
		case "launcher":
			launcher = true
		}
	}
	// Only tuple.owner_inputs has the three allOf/contains constraints. Neither
	// shape has uniqueItems; catalogEntry deliberately has no contains rule.
	return !requireEachOwner || curator && runtime && launcher
}
func managerValid(v any) bool {
	o := object(v)
	switch o["mode"] {
	case "required":
		return closed(o, "mode", "id", "registration_sha256", "transport_binding_sha256", "capabilities_sha256", "protocol", "compatibility_sha256") && name(o["id"]) && digest(o["registration_sha256"]) && digest(o["transport_binding_sha256"]) && digest(o["capabilities_sha256"]) && textLength(o["protocol"], 128) && digest(o["compatibility_sha256"])
	case "compatible_absence":
		return closed(o, "mode", "compatibility_sha256") && digest(o["compatibility_sha256"])
	default:
		return false
	}
}
func effortValid(v any) bool {
	o := object(v)
	switch o["mode"] {
	case "named":
		return closed(o, "mode", "value") && textLength(o["value"], 64)
	case "not_applicable":
		return closed(o, "mode", "evidence_sha256") && digest(o["evidence_sha256"])
	default:
		return false
	}
}
func runtimeValid(v any) bool {
	o := object(v)
	return closed(o, "id", "vendor", "release", "binary_sha256", "model", "effort", "os", "architecture", "adapter_id", "adapter_revision") && name(o["id"]) && name(o["vendor"]) && textLength(o["release"], 128) && digest(o["binary_sha256"]) && textLength(o["model"], 128) && effortValid(o["effort"]) && is(o["os"], "linux", "darwin") && is(o["architecture"], "amd64", "arm64") && name(o["adapter_id"]) && revision(o["adapter_revision"])
}
func profileValid(v any) bool {
	o := object(v)
	return closed(o, "id", "revision", "source_closure_sha256", "config_sha256", "lock_sha256", "fragment_kind", "fragment_sha256", "effective_config_sha256", "permission_posture", "permission_policy_sha256") && name(o["id"]) && revision(o["revision"]) && digest(o["source_closure_sha256"]) && digest(o["config_sha256"]) && digest(o["lock_sha256"]) && textLength(o["fragment_kind"], 128) && digest(o["fragment_sha256"]) && digest(o["effective_config_sha256"]) && textLength(o["permission_posture"], 64) && digest(o["permission_policy_sha256"])
}
func homeValid(v any) bool {
	o := object(v)
	return closed(o, "uid", "mode", "provenance_sha256", "native_origin_sha256", "auth_binding_sha256") && integer(o["uid"], 1, 4294967295) && is(o["mode"], "managed_snapshot", "native_strict") && digest(o["provenance_sha256"]) && digest(o["native_origin_sha256"]) && digest(o["auth_binding_sha256"])
}
func tupleValid(o map[string]any) bool {
	if !closed(o, "kind", "launcher", "curator", "agents_management", "runtime", "profile", "home", "manager", "owner_inputs") || !is(o["kind"], "curator-run member-tuple/1") {
		return false
	}
	sam := object(o["agents_management"])
	return componentValid(o["launcher"]) && componentValid(o["curator"]) && closed(sam, "module", "version", "revision") && is(sam["module"], "github.com/relux-works/skill-agents-management") && textLength(sam["version"], 128) && revision(sam["revision"]) && runtimeValid(o["runtime"]) && profileValid(o["profile"]) && homeValid(o["home"]) && managerValid(o["manager"]) && ownerInputsValid(o["owner_inputs"], true)
}
func ownerValid(o map[string]any) bool {
	return closed(o, "kind", "controller_id", "store_id", "uid", "catalog_sha256") && is(o["kind"], "curator-run member-owner/1") && id(o["controller_id"]) && id(o["store_id"]) && integer(o["uid"], 1, 4294967295) && digest(o["catalog_sha256"])
}
func catalogValid(o map[string]any) bool {
	if !closed(o, "kind", "controller_id", "store_id", "entries") || !is(o["kind"], "curator-run member-catalog/1") || !id(o["controller_id"]) || !id(o["store_id"]) {
		return false
	}
	a, ok := o["entries"].([]any)
	if !ok || len(a) > 32 {
		return false
	}
	for _, item := range a {
		e := object(item)
		if !closed(e, "tuple", "owner_inputs", "construction_contract_sha256", "private_verification_contract_sha256", "scope_contract_sha256") || !reference(e["tuple"]) || !ownerInputsValid(e["owner_inputs"], false) || !digest(e["construction_contract_sha256"]) || !digest(e["private_verification_contract_sha256"]) || !digest(e["scope_contract_sha256"]) {
			return false
		}
	}
	return true
}
