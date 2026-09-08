package composition

import (
	"errors"
	"fmt"
	"os"
)

const (
	CodeMCPLayerMissing    = "mcp_layer_missing"
	CodeMCPLayerUnreadable = "mcp_layer_unreadable"
)

// LayerError distinguishes actual absence from a failed or nonregular read.
type LayerError struct {
	Code string
	Path string
	Err  error
}

func (e *LayerError) Error() string { return fmt.Sprintf("%s: %s: %v", e.Code, e.Path, e.Err) }
func (e *LayerError) Unwrap() error { return e.Err }

// CheckLaunchBoundary probes the codex MCP layer afresh on every invocation.
// The execution Story MUST call this immediately before BOTH direct process
// creation and ax handoff, alongside binary and §5 file-kind checks. Compose
// deliberately never calls it: a successful early check is stale evidence.
// This read-only check never removes flags, repairs files, or falls back.
// As with any pathname probe, a replacement after this check remains a TOCTOU
// bound; execution must minimize that window, not treat this as an open handle.
func (v Value) CheckLaunchBoundary() error {
	if v.mcpLayer == "" {
		return nil
	}
	path := v.mcpLayer
	fail := func(code string, err error) error { return &LayerError{Code: code, Path: path, Err: err} }
	_, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fail(CodeMCPLayerMissing, err)
		}
		return fail(CodeMCPLayerUnreadable, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fail(CodeMCPLayerUnreadable, err)
	}
	if !info.Mode().IsRegular() {
		return fail(CodeMCPLayerUnreadable, errors.New("not a regular file"))
	}
	f, err := os.Open(path)
	if err != nil {
		return fail(CodeMCPLayerUnreadable, err)
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return fail(CodeMCPLayerUnreadable, err)
	}
	if !info.Mode().IsRegular() {
		return fail(CodeMCPLayerUnreadable, errors.New("opened file is not regular"))
	}
	return nil
}
