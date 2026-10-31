package network

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/relux-works/curator-network-profiles/pkg/adapterprobe"
	"github.com/relux-works/curator-network-profiles/pkg/envpatch"
	"github.com/relux-works/curator-network-profiles/pkg/refusal"
)

// QualifiedFileName is the operator's list of harness builds qualified for
// sensitive egress, beside network.toml under $HOME/.curator. Strict
// admission (a sensitive_egress profile) consults only this list; a build
// is named by the sha256 of its executable, never by its version output.
const QualifiedFileName = "network-qualified.toml"

// QualifiedSchema is the only accepted `schema` of the file.
const QualifiedSchema = "curator-network-qualified-v1"

type qualifiedFile struct {
	Schema string           `toml:"schema"`
	Builds []qualifiedBuild `toml:"builds"`
	// ProjectRoots are the directories a filtering-gateway launch may run
	// in (admission contract A): its physical working directory must be one
	// of them or below one.
	ProjectRoots []string `toml:"project_roots"`
}

type qualifiedBuild struct {
	Harness      string `toml:"harness"`
	Entrypoint   string `toml:"entrypoint"`
	BinarySHA256 string `toml:"binary_sha256"`
	Platform     string `toml:"platform"`
	Evidence     string `toml:"evidence"`
	QualifiedOn  string `toml:"qualified_on"`
}

var (
	sha256RE   = regexp.MustCompile(`^[0-9a-f]{64}$`)
	platformRE = regexp.MustCompile(`^[a-z0-9]{1,32}$`)
)

// recipeFor is the qualification recipe Admit evaluates for a harness. A
// filtering gateway evaluates Claude Code's interactive session under its
// own recipe; every other launch keeps the exec recipe as before.
func recipeFor(harness, entrypoint string, gateway bool) string {
	switch harness {
	case "codex-cli":
		return "codex-exec-v1"
	case "muse":
		return "muse-exec-v1"
	}
	if gateway && entrypoint == EntrypointInteractive {
		return "claude-interactive-v1"
	}
	return "claude-exec-v1"
}

// loadQualified reads the operator's qualified builds for this platform.
// An absent file is an empty list, so strict admission refuses as before;
// a file that exists but cannot be read or does not validate refuses, so a
// broken list never widens or silently narrows admission.
func loadQualified(read func(string) ([]byte, error), curatorDir string) ([]adapterprobe.AllowedBuild, error) {
	if curatorDir == "" {
		return nil, nil
	}
	data, err := read(filepath.Join(curatorDir, QualifiedFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, refusal.New(refusal.CodeFileUnreadable, QualifiedFileName, "the qualified-builds file is not readable")
	}
	invalid := func(detail string) error {
		return refusal.New(refusal.CodeProfileInvalid, QualifiedFileName, detail)
	}
	var file qualifiedFile
	dec := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields()
	if err := dec.Decode(&file); err != nil {
		return nil, invalid("the qualified-builds file does not decode")
	}
	if file.Schema != QualifiedSchema {
		return nil, invalid("schema must be \"" + QualifiedSchema + "\"")
	}
	var out []adapterprobe.AllowedBuild
	for _, b := range file.Builds {
		switch {
		// Claude Code is the only harness the filtering gateway binds in
		// this revision; another harness enters with its own shape.
		case b.Harness != claudeCodeHarness:
			return nil, invalid("harness must be claude-code")
		case b.Entrypoint != EntrypointExec && b.Entrypoint != EntrypointInteractive:
			return nil, invalid("entrypoint must be exec or interactive")
		case !sha256RE.MatchString(b.BinarySHA256):
			return nil, invalid("binary_sha256 must be 64 lowercase hex characters")
		case !platformRE.MatchString(b.Platform):
			return nil, invalid("platform must be a lowercase GOOS name")
		case len(b.Evidence) == 0 || len(b.Evidence) > 512 || !printable(b.Evidence):
			return nil, invalid("evidence must name the qualification record in 1 to 512 printable characters")
		}
		if _, err := time.Parse("2006-01-02", b.QualifiedOn); err != nil {
			return nil, invalid("qualified_on must be a YYYY-MM-DD date")
		}
		if b.Platform != runtime.GOOS {
			continue
		}
		build, err := adapterprobe.BuildID(b.BinarySHA256)
		if err != nil {
			return nil, invalid("binary_sha256 is not a build digest")
		}
		out = append(out, adapterprobe.AllowedBuild{
			Adapter:  adapterprobe.Identity{Adapter: envpatch.AdapterGeneric, Harness: b.Harness, Entrypoint: b.Entrypoint},
			BuildID:  build,
			Recipe:   recipeFor(b.Harness, b.Entrypoint, true),
			Platform: b.Platform,
			Scope:    adapterprobe.TransportScope,
		})
	}
	return out, nil
}

func printable(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < ' ' || s[i] > '~' {
			return false
		}
	}
	return true
}

// loadGatewayRoots reads the operator's project roots for filtering-gateway
// launches from the qualified-builds file. Each must be an absolute, clean
// path that exists; it is returned resolved to its physical path. A missing
// file or list is an empty list, which admits no gateway launch.
func loadGatewayRoots(read func(string) ([]byte, error), curatorDir string) ([]string, error) {
	if curatorDir == "" {
		return nil, nil
	}
	data, err := read(filepath.Join(curatorDir, QualifiedFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, refusal.New(refusal.CodeFileUnreadable, QualifiedFileName, "the qualified-builds file is not readable")
	}
	var file qualifiedFile
	dec := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields()
	if err := dec.Decode(&file); err != nil || file.Schema != QualifiedSchema {
		return nil, refusal.New(refusal.CodeProfileInvalid, QualifiedFileName, "the qualified-builds file does not decode")
	}
	var roots []string
	for _, root := range file.ProjectRoots {
		if !filepath.IsAbs(root) || filepath.Clean(root) != root {
			return nil, refusal.New(refusal.CodeProfileInvalid, QualifiedFileName, "project_roots must be absolute clean paths")
		}
		physical, err := filepath.EvalSymlinks(root)
		if err != nil {
			return nil, refusal.New(refusal.CodeProfileInvalid, QualifiedFileName, "a project root does not resolve")
		}
		roots = append(roots, physical)
	}
	return roots, nil
}
