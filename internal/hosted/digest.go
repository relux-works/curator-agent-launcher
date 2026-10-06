package hosted

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/relux-works/curator-agent-launcher/internal/fragment"
)

// ContentDigest returns "sha256:<hex>" over the CCJ-1 bytes of the payload
// object excluding only its top-level content_digest member. The member is
// stripped when present, so the same function digests a builder object and
// re-verifies a wire object the way the receiver recomputes it.
func ContentDigest(payloadObject []byte) (string, error) {
	v, err := fragment.ParseJSON(payloadObject)
	if err != nil {
		return "", &PlanError{Code: CodeLaunchPlanInvalid, Field: "payload", Reason: "payload is not CCJ-1 JSON"}
	}
	if v.Kind != fragment.KindObject {
		return "", &PlanError{Code: CodeLaunchPlanInvalid, Field: "payload", Reason: "payload must be an object"}
	}
	stripped := fragment.Value{Kind: fragment.KindObject}
	for _, m := range v.Obj {
		if m.Key == "content_digest" {
			continue
		}
		stripped.Obj = append(stripped.Obj, m)
	}
	sum := sha256.Sum256(fragment.Canonical(stripped))
	return fragment.DigestPrefix + hex.EncodeToString(sum[:]), nil
}
