package launcherconfig

import (
	"regexp"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

// Notification contains Core-selected immutable workflow metadata. It conveys
// no member admission, runtime capability, endpoint or enrollment authority.
type Notification struct {
	Backend           string `json:"backend"`
	Contract          string `json:"contract,omitempty"`
	WorkflowReference string `json:"workflow_reference,omitempty"`
	WorkflowSHA256    string `json:"workflow_sha256,omitempty"`
}

var notificationReference = regexp.MustCompile(`^[A-Za-z0-9._:/@+-]{1,128}$`)
var notificationDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

func parseNotification(v fragment.Value) (*Notification, error) {
	switch stringMember(v, "backend") {
	case "off":
		if !exactMembers(v, "backend") {
			return nil, ErrSelected
		}
		return &Notification{Backend: "off"}, nil
	case "host":
		if !exactMembers(v, "backend", "contract", "workflow_reference", "workflow_sha256") || stringMember(v, "contract") != "curator-host-notification/1" {
			return nil, ErrSelected
		}
		ref, digest := stringMember(v, "workflow_reference"), stringMember(v, "workflow_sha256")
		if !notificationReference.MatchString(ref) || !notificationDigest.MatchString(digest) {
			return nil, ErrSelected
		}
		return &Notification{Backend: "host", Contract: "curator-host-notification/1", WorkflowReference: ref, WorkflowSHA256: digest}, nil
	default:
		return nil, ErrSelected
	}
}
