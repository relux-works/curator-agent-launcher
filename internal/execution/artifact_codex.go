package execution

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/relux-works/curator-network-profiles/pkg/adapterprobe"
)

// Codex resolves its code-mode host, bundled tools and resources relative to
// current_exe. Preserve that distribution layout rather than moving one binary.
// These bounds cover the shipped native package without traversing an install
// root, following links, or accepting unbounded resource trees.
const maxPackageBytes int64 = 1 << 30
const maxPackageFiles = 4096

type packageFile struct {
	Digest [32]byte
	Mode   fs.FileMode
	Size   int64
}

// PackageReceipt describes captured distribution bytes, not release authenticity
// or adapter qualification. Paths are package-relative and contain no home data.
type PackageReceipt struct {
	Layout, Entrypoint, Host, Version, Target, MainBuild, AdmittedRelease string
	Members                                                               []PackageMember
}

type PackageMember struct {
	Path, Role, SHA256 string
	Mode               fs.FileMode
	Size               int64
}

func (s *ArtifactSnapshot) PackageReceipt() PackageReceipt {
	r := s.receipt
	r.Members = append([]PackageMember(nil), r.Members...)
	return r
}

// SnapshotCodexArtifact retains the main executable's adapter identity and seals
// its effective runtime package separately. Package integrity does not qualify a
// companion as a separately approved adapter build.
func SnapshotCodexArtifact(binary, workdir string, env []string, release string) (*ArtifactSnapshot, error) {
	path, err := executable(binary, workdir, env)
	if err != nil {
		return nil, packageRefused()
	}
	if filepath.Base(filepath.Dir(path)) == "MacOS" && filepath.Base(filepath.Dir(filepath.Dir(path))) == "Contents" {
		return nil, packageRefused()
	}
	entryInfo, err := os.Lstat(path)
	if err != nil || !entryInfo.Mode().IsRegular() {
		return nil, packageRefused()
	}
	if filepath.Base(filepath.Dir(path)) == "bin" {
		if info, err := os.Lstat(filepath.Dir(path)); err != nil || !info.IsDir() {
			return nil, packageRefused()
		}
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, packageRefused()
	}
	root, entry := filepath.Dir(path), filepath.Base(path)
	if filepath.Base(root) == "bin" {
		candidate := filepath.Join(filepath.Dir(root), "codex-package.json")
		if _, err := os.Lstat(candidate); err == nil {
			root, entry = filepath.Dir(root), filepath.Join("bin", entry)
		} else if !os.IsNotExist(err) {
			return nil, packageRefused()
		}
	}
	paths := []string{entry}
	if filepath.Base(entry) != "codex" && filepath.Base(entry) != "codex.exe" {
		return nil, packageRefused()
	}
	rooted, err := os.OpenRoot(root)
	if err != nil {
		return nil, packageRefused()
	}
	defer rooted.Close()
	// All source opens are rooted: even a concurrent directory replacement
	// cannot redirect a resource read outside the admitted package root.
	_, manifestErr := rooted.Lstat("codex-package.json")
	var metadataBytes []byte
	receipt := PackageReceipt{Layout: "legacy-sibling", Entrypoint: filepath.ToSlash(entry), AdmittedRelease: release}
	if manifestErr == nil {
		info, err := rooted.Lstat("codex-package.json")
		if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<10 {
			return nil, packageRefused()
		}
		f, err := rooted.Open("codex-package.json")
		if err != nil {
			return nil, packageRefused()
		}
		data, err := readPackageFile(f, 64<<10)
		f.Close()
		metadataBytes = data
		var metadata struct {
			LayoutVersion int    `json:"layoutVersion"`
			Entrypoint    string `json:"entrypoint"`
			ResourcesDir  string `json:"resourcesDir"`
			PathDir       string `json:"pathDir"`
			Version       string `json:"version"`
			Target        string `json:"target"`
		}
		if err != nil || json.Unmarshal(data, &metadata) != nil || metadata.LayoutVersion != 1 || metadata.Version != release || metadata.Entrypoint != filepath.ToSlash(entry) ||
			(metadata.ResourcesDir != "" && metadata.ResourcesDir != "codex-resources") || (metadata.PathDir != "" && metadata.PathDir != "codex-path") {
			return nil, packageRefused()
		}
		receipt.Layout, receipt.Version, receipt.Target = "package-v1", metadata.Version, metadata.Target
		paths = append(paths, "codex-package.json")
		// Codex discovers these fixed directories independently of manifest
		// values. Include only the directories its normal route can discover.
		for _, name := range []string{"codex-resources", "codex-path"} {
			if info, err := rooted.Lstat(name); err == nil {
				if !info.IsDir() {
					return nil, packageRefused()
				}
				paths = append(paths, name)
			} else if !os.IsNotExist(err) {
				return nil, packageRefused()
			}
		}
	} else if !os.IsNotExist(manifestErr) {
		return nil, packageRefused()
	}
	// Match InstallContext::code_mode_host_program: resources first, then
	// the executable's directory. A standalone binary needs its sibling too.
	host := "codex-code-mode-host"
	if strings.HasSuffix(entry, ".exe") {
		host += ".exe"
	}
	selected := filepath.Join(filepath.Dir(entry), host)
	if manifestErr == nil {
		resource := filepath.Join("codex-resources", host)
		if _, err := rooted.Lstat(resource); err == nil {
			selected = resource
		} else if !os.IsNotExist(err) {
			return nil, packageRefused()
		}
	}
	info, err := rooted.Lstat(selected)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return nil, packageRefused()
	}
	hostInfo := info
	receipt.Host = filepath.ToSlash(selected)
	if selected != filepath.Join("codex-resources", host) {
		paths = append(paths, selected)
	}
	dir, err := os.MkdirTemp("", "curator-adapter-")
	if err != nil {
		return nil, packageRefused()
	}
	s := &ArtifactSnapshot{dir: dir, path: filepath.Join(dir, entry), files: map[string]packageFile{}, dirs: map[string]fs.FileMode{}, receipt: receipt}
	ok := false
	defer func() {
		if !ok {
			s.Close()
		}
	}()
	var total int64
	entries := 0
	for _, name := range paths {
		for parent := filepath.Dir(name); parent != "."; parent = filepath.Dir(parent) {
			info, err := rooted.Lstat(parent)
			if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return nil, packageRefused()
			}
		}
		err := fs.WalkDir(rooted.FS(), filepath.ToSlash(name), func(source string, ent fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			entries++
			if entries > maxPackageFiles {
				return packageRefused()
			}
			rel := filepath.FromSlash(source)
			if !filepath.IsLocal(rel) || strings.Count(rel, string(filepath.Separator)) > 32 || ent.Type()&os.ModeSymlink != 0 {
				return packageRefused()
			}
			info, err := ent.Info()
			if err != nil {
				return err
			}
			target := filepath.Join(dir, rel)
			if ent.IsDir() {
				return os.MkdirAll(target, 0700)
			}
			if info.Size() > adapterprobe.MaxArtifactBytes {
				if rel == entry {
					return &adapterprobe.Unsupported{Reason: "artifact_too_large"}
				}
				return packageRefused()
			}
			if !info.Mode().IsRegular() || len(s.files) >= maxPackageFiles || info.Size() < 0 || info.Size() > maxPackageBytes-total || ((rel == entry || rel == selected) && info.Mode().Perm()&0111 == 0) {
				return packageRefused()
			}
			if (rel == entry && !os.SameFile(entryInfo, info)) || (rel == selected && !os.SameFile(hostInfo, info)) {
				return packageRefused()
			}
			// A post-open identity check rejects a replaced or linked source.
			f, err := rooted.Open(rel)
			if err != nil {
				return err
			}
			opened, statErr := f.Stat()
			current, linkErr := rooted.Lstat(rel)
			if statErr != nil || linkErr != nil || !current.Mode().IsRegular() || !os.SameFile(info, opened) || !os.SameFile(opened, current) {
				f.Close()
				return packageRefused()
			}
			limit := maxPackageBytes - total
			if rel == entry && limit > adapterprobe.MaxArtifactBytes {
				limit = adapterprobe.MaxArtifactBytes
			}
			data, readErr := readPackageFile(f, limit)
			after, afterErr := f.Stat()
			f.Close()
			if readErr != nil || afterErr != nil || !os.SameFile(opened, after) || opened.Size() != after.Size() || opened.Mode() != after.Mode() || !opened.ModTime().Equal(after.ModTime()) || int64(len(data)) != opened.Size() {
				return packageRefused()
			}
			if rel == "codex-package.json" && string(data) != string(metadataBytes) {
				return packageRefused()
			}
			total += int64(len(data))
			mode := fs.FileMode(0400)
			if info.Mode().Perm()&0111 != 0 {
				mode = 0500
			}
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return err
			}
			if err := os.WriteFile(target, data, mode); err != nil {
				return err
			}
			s.files[rel] = packageFile{sha256.Sum256(data), mode, int64(len(data))}
			role := "resource"
			switch rel {
			case entry:
				role = "entrypoint"
			case selected:
				role = "code-mode-host"
			case "codex-package.json":
				role = "layout-metadata"
			}
			s.receipt.Members = append(s.receipt.Members, PackageMember{Path: filepath.ToSlash(rel), Role: role, SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), Mode: mode, Size: int64(len(data))})
			if rel == entry {
				s.data = data
			}
			return nil
		})
		if err != nil {
			if unsupported, ok := err.(*adapterprobe.Unsupported); ok && unsupported.Reason == "artifact_too_large" {
				return nil, err
			}
			return nil, packageRefused()
		}
	}
	if len(s.data) > adapterprobe.MaxArtifactBytes {
		return nil, &adapterprobe.Unsupported{Reason: "artifact_too_large"}
	}
	s.build, err = adapterprobe.BuildID(fmt.Sprintf("%x", sha256.Sum256(s.data)))
	s.receipt.MainBuild = s.build
	sort.Slice(s.receipt.Members, func(i, j int) bool { return s.receipt.Members[i].Path < s.receipt.Members[j].Path })
	if err := filepath.WalkDir(s.dir, func(path string, ent fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ent.IsDir() {
			rel, err := filepath.Rel(s.dir, path)
			if err != nil {
				return err
			}
			s.dirs[rel] = 0700
		}
		if len(s.dirs)+len(s.files) > maxPackageFiles {
			return packageRefused()
		}
		return nil
	}); err != nil {
		return nil, packageRefused()
	}
	if err != nil || s.Verify() != nil {
		return nil, packageRefused()
	}
	ok = true
	return s, nil
}

func readPackageFile(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, packageRefused()
	}
	return data, nil
}

func packageRefused() error {
	return &adapterprobe.Unsupported{Reason: "artifact_package_invalid"}
}

// Verify rechecks the sealed package, including added files and mode changes,
// at the final process boundary. It never reads the original installation.
func (s *ArtifactSnapshot) Verify() error {
	if s == nil || s.files == nil {
		return nil
	}
	actual := map[string]packageFile{}
	dirs := map[string]fs.FileMode{}
	err := filepath.WalkDir(s.dir, func(path string, ent fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := ent.Info()
		if err != nil || ent.Type()&os.ModeSymlink != 0 {
			return packageRefused()
		}
		rel, err := filepath.Rel(s.dir, path)
		if err != nil {
			return err
		}
		if ent.IsDir() {
			dirs[rel] = info.Mode().Perm()
			return nil
		}
		if !info.Mode().IsRegular() {
			return packageRefused()
		}
		expected, exists := s.files[rel]
		if !exists || info.Size() != expected.Size || info.Mode().Perm() != expected.Mode || len(actual) >= maxPackageFiles {
			return packageRefused()
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		data, err := readPackageFile(f, expected.Size)
		f.Close()
		if err != nil {
			return err
		}
		actual[rel] = packageFile{sha256.Sum256(data), info.Mode().Perm(), int64(len(data))}
		return nil
	})
	if err != nil || !reflect.DeepEqual(actual, s.files) || !reflect.DeepEqual(dirs, s.dirs) {
		return packageRefused()
	}
	return nil
}
