//go:build !darwin && !linux

package memberobjects

import "os"

type retained struct{}

func acquirePlatform(_ *os.File, _ []Reference) (*Collection, *Error) { return nil, descriptorError() }
func (*retained) recheck(map[string]*Snapshot) *Error                 { return descriptorError() }
func (*retained) close() *Error                                       { return nil }
