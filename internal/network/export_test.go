package network

import "os"

// SetOpenSettingsFile replaces how settings sources are opened, for tests
// that stage a slow open or a replacement; restore puts the original back.
// Tests that use it must not run in parallel.
func SetOpenSettingsFile(open func(string) (*os.File, error)) (restore func()) {
	previous := openSettingsFile
	openSettingsFile = open
	return func() { openSettingsFile = previous }
}

// productionOpenSettingsFile is the opener as built, captured before any
// test replaces it.
var productionOpenSettingsFile = openSettingsFile

// OpenSettingsFile is the production opener, for wrapping.
func OpenSettingsFile(path string) (*os.File, error) { return productionOpenSettingsFile(path) }

// SetInspectPath replaces how settings and policy paths are inspected.
func SetInspectPath(inspect func(string) (os.FileInfo, error)) (restore func()) {
	previous := inspectPath
	inspectPath = inspect
	return func() { inspectPath = previous }
}

// SetListDropIns replaces how a managed drop-in directory is listed.
func SetListDropIns(list func(string, int) ([]os.DirEntry, error)) (restore func()) {
	previous := listDropIns
	listDropIns = list
	return func() { listDropIns = previous }
}

// ListDropIns is the production lister, for wrapping.
func ListDropIns(dir string, n int) ([]os.DirEntry, error) { return productionListDropIns(dir, n) }

var productionListDropIns = listDropIns
