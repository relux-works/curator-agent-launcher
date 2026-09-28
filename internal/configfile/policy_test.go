package configfile

import "testing"

func TestForeignDACLWriteGrantPolicy(t *testing.T) {
	const writeMask = 0x40000000
	for _, tc := range []struct {
		name, trustee string
		mask          uint32
		want          bool
	}{
		{"owner-write", "S-1-5-21-1", writeMask, false},
		{"owner-rights-write", ownerRightsSID, writeMask, false},
		{"creator-owner-write", creatorOwnerSID, writeMask, false},
		{"system-write", localSystemSID, writeMask, false},
		{"administrators-write", builtinAdministrators, writeMask, false},
		{"other-user-write", "S-1-5-21-2", writeMask, true},
		{"everyone-write", "S-1-1-0", writeMask, true},
		{"other-user-read-only", "S-1-5-21-2", 0x80000000, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasForeignWriteGrant(tc.mask, writeMask, tc.trustee, "S-1-5-21-1"); got != tc.want {
				t.Fatalf("hasForeignWriteGrant = %v, want %v", got, tc.want)
			}
		})
	}
}
