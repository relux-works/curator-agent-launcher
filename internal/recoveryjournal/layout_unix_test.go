//go:build darwin || linux

package recoveryjournal

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/sys/unix"
)

func fixtureBinding(t *testing.T) Binding {
	t.Helper()
	// macOS's /var is itself a symlink. Resolve only this test-owned fixture
	// path; production opens every selected component without following links.
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(parent, "state")
	if err := os.Mkdir(base, 0700); err != nil {
		t.Fatal(err)
	}
	in := trustedInputs()
	in.XDGStateHome = base
	b, err := Resolve(in)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func fixtureLayout(t *testing.T, b Binding) {
	t.Helper()
	for _, path := range []string{b.Directory(), filepath.Join(b.Directory(), "secrets"), filepath.Join(b.Directory(), "receipts")} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
}

// A real private layout opens successfully, stays descriptor-bound across a
// namespace rename, and Close releases each descriptor exactly once.
func TestOpenExistingNamespace(t *testing.T) {
	b := fixtureBinding(t)
	fixtureLayout(t, b)
	// Non-private original state/container directories are legitimate. Only
	// the versioned namespace and its secret/receipt children must be 0700.
	for _, path := range []string{b.StateBase(), filepath.Join(b.StateBase(), "swarma"), filepath.Join(b.StateBase(), "swarma/launcher"), filepath.Join(b.StateBase(), "swarma/launcher/netproxy-recovery")} {
		if err := os.Chmod(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	n, err := OpenExisting(b)
	if err != nil {
		t.Fatal(err)
	}
	fds := []int{n.base, n.directory, n.secrets, n.receipts}
	if err := os.Rename(b.Directory(), b.Directory()+"-moved"); err != nil {
		t.Fatal(err)
	}
	for _, fd := range fds {
		if err := unix.Fstat(fd, new(unix.Stat_t)); err != nil {
			t.Fatal("retained descriptor lost")
		}
	}
	if err := n.Close(); err != nil {
		t.Fatal(err)
	}
	if err := n.Close(); err != nil {
		t.Fatal("second close failed")
	}
	for _, fd := range fds {
		if err := unix.Fstat(fd, new(unix.Stat_t)); !errors.Is(err, unix.EBADF) {
			t.Fatal("descriptor leaked")
		}
	}
}

// Missing original state parents are explicitly included, and both missing
// children are reported together. Neither case creates a replacement base.
func TestOpenMissingBaseAndChildren(t *testing.T) {
	b := fixtureBinding(t)
	in := trustedInputs()
	in.XDGStateHome = filepath.Join(b.StateBase(), "missing", "state")
	missingBase, err := Resolve(in)
	if err != nil {
		t.Fatal(err)
	}
	_, err = OpenExisting(missingBase)
	var missing *MissingDirectories
	if !errors.As(err, &missing) {
		t.Fatalf("missing base not reported: %v", err)
	}
	if got := missing.Targets(); len(got) != 8 || got[0] != filepath.Join(b.StateBase(), "missing") || got[1] != missingBase.StateBase() || got[len(got)-1] != filepath.Join(missingBase.Directory(), "receipts") {
		t.Fatal("missing base write closure incomplete")
	}
	fixtureLayout(t, b)
	for _, name := range []string{"secrets", "receipts"} {
		if err := os.Remove(filepath.Join(b.Directory(), name)); err != nil {
			t.Fatal(err)
		}
	}
	_, err = OpenExisting(b)
	if !errors.As(err, &missing) || !reflect.DeepEqual(missing.Targets(), []string{filepath.Join(b.Directory(), "secrets"), filepath.Join(b.Directory(), "receipts")}) {
		t.Fatal("missing children not reported together")
	}
	if _, err := OpenExisting(Binding{}); !errors.Is(err, ErrBinding) {
		t.Fatal("zero original binding accepted")
	}
}

// Missing directories are a concrete future write-set, never MkdirAll side
// effects; callers must obtain canonical preflight/adoption before creation.
func TestOpenReportsMissingClosureWithoutWrites(t *testing.T) {
	b := fixtureBinding(t)
	n, err := OpenExisting(b)
	var missing *MissingDirectories
	if n != nil || !errors.As(err, &missing) {
		t.Fatalf("missing layout not reported: %v", err)
	}
	want := []string{
		filepath.Join(b.StateBase(), "swarma"),
		filepath.Join(b.StateBase(), "swarma/launcher"),
		filepath.Join(b.StateBase(), "swarma/launcher/netproxy-recovery"),
		b.Directory(), filepath.Join(b.Directory(), "secrets"), filepath.Join(b.Directory(), "receipts"),
	}
	if !reflect.DeepEqual(missing.Targets(), want) {
		t.Fatal("incomplete or unrelated missing write closure")
	}
	copy := missing.Targets()
	copy[0] = "/unrelated"
	if !reflect.DeepEqual(missing.Targets(), want) {
		t.Fatal("mutable error write closure")
	}
	entries, err := os.ReadDir(b.StateBase())
	if err != nil || len(entries) != 0 {
		t.Fatal("preflight created state")
	}
	if missing.Error() != "recovery journal: directories require admitted creation" {
		t.Fatal("missing error discloses private paths")
	}
}

// Every selected component is no-follow. Neither an ancestor link, a final
// directory link, nor a private child's link can redirect reads or writes.
func TestOpenRefusesLinkedComponents(t *testing.T) {
	for _, component := range []string{"base", "swarma", "launcher", "netproxy-recovery", "v1", "secrets", "receipts"} {
		t.Run(component, func(t *testing.T) {
			b := fixtureBinding(t)
			fixtureLayout(t, b)
			path := b.StateBase()
			if component != "base" {
				for _, part := range []string{"swarma", "launcher", "netproxy-recovery", "v1", "secrets"} {
					if component == "receipts" && part == "secrets" {
						part = "receipts"
					}
					path = filepath.Join(path, part)
					if part == component {
						break
					}
				}
			}
			if err := os.Rename(path, path+"-sentinel"); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(path+"-sentinel", path); err != nil {
				t.Fatal(err)
			}
			n, err := OpenExisting(b)
			if n != nil || !errors.Is(err, ErrUnsafeLayout) {
				t.Fatalf("linked component accepted: %v", err)
			}
			if st, err := os.Lstat(path); err != nil || st.Mode()&os.ModeSymlink == 0 {
				t.Fatal("guard repaired the path")
			}
		})
	}
}

// Correct owner and mode are positive controls for private directories.
// A file's nlink==1 predicate is intentionally not imposed on directories.
func TestDirectoryPredicates(t *testing.T) {
	uid := uint32(os.Geteuid())
	valid := unix.Stat_t{Uid: uid, Mode: unix.S_IFDIR | 0700, Nlink: 4}
	if !validDirectory(valid, uid, true) {
		t.Fatal("ordinary private directory rejected")
	}
	for _, tc := range []struct {
		name string
		edit func(*unix.Stat_t)
	}{
		{"foreign_owner", func(s *unix.Stat_t) { s.Uid++ }},
		{"regular_file", func(s *unix.Stat_t) { s.Mode = unix.S_IFREG | 0700 }},
		{"permissive", func(s *unix.Stat_t) { s.Mode = unix.S_IFDIR | 0755 }},
		{"setgid", func(s *unix.Stat_t) { s.Mode |= unix.S_ISGID }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := valid
			tc.edit(&s)
			if validDirectory(s, uid, true) {
				t.Fatal("unsafe directory accepted")
			}
		})
	}
}

// Existing unsafe nodes refuse without mutation; a missing secrets directory
// does not hide an unsafe receipts node later in the selected layout.
func TestOpenRefusesUnsafePrivateNodes(t *testing.T) {
	for _, node := range []string{"v1", "secrets", "receipts"} {
		for _, kind := range []string{"permissive", "file", "fifo"} {
			t.Run(node+"/"+kind, func(t *testing.T) {
				b := fixtureBinding(t)
				fixtureLayout(t, b)
				path := b.Directory()
				if node != "v1" {
					path = filepath.Join(path, node)
				}
				if kind == "permissive" {
					if err := os.Chmod(path, 0755); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := os.Rename(path, path+"-sentinel"); err != nil {
						t.Fatal(err)
					}
					if kind == "file" {
						if err := os.WriteFile(path, []byte("unchanged"), 0600); err != nil {
							t.Fatal(err)
						}
					} else if err := unix.Mkfifo(path, 0600); err != nil {
						t.Fatal(err)
					}
				}
				before, _ := os.Lstat(path)
				n, err := OpenExisting(b)
				if n != nil || !errors.Is(err, ErrUnsafeLayout) {
					t.Fatalf("unsafe private node accepted: %v", err)
				}
				after, e := os.Lstat(path)
				if e != nil || before.Mode() != after.Mode() || !os.SameFile(before, after) {
					t.Fatal("guard mutated unsafe node")
				}
			})
		}
	}
	b := fixtureBinding(t)
	fixtureLayout(t, b)
	if err := os.Remove(filepath.Join(b.Directory(), "secrets")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(b.Directory(), "receipts"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenExisting(b); !errors.Is(err, ErrUnsafeLayout) {
		t.Fatalf("missing node concealed unsafe sibling: %v", err)
	}
}
