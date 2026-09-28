package configfile

const (
	ownerRightsSID        = "S-1-3-4"
	creatorOwnerSID       = "S-1-3-0"
	localSystemSID        = "S-1-5-18"
	builtinAdministrators = "S-1-5-32-544"
)

func hasForeignWriteGrant(mask, writeMask uint32, trusteeSID, ownerSID string) bool {
	if mask&writeMask == 0 {
		return false
	}
	switch trusteeSID {
	case ownerSID, ownerRightsSID, creatorOwnerSID, localSystemSID, builtinAdministrators:
		return false
	default:
		return true
	}
}
