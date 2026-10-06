// Fake task-board receiver for hosted transport tests. It captures argv,
// stdin, env, and fd3 bytes, replays framed fd4 records from a file, and
// exits with a controlled status. Control via environment:
//
//	RECEIVER_CAPTURE_DIR  directory for argv.json, stdin.bin, env.json, fd3.txt (required)
//	RECEIVER_RECORDS      file with one JSON object per line, framed to fd4 in order (optional)
//	RECEIVER_CORRUPT      garbage|oversize|truncated|badjson|zero|partial-header|partial-body:
//	                      corrupt suffix bytes after any records (mixed-frame shapes)
//	RECEIVER_EXIT         exit status (default 0)
package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

func fail(err error) {
	fmt.Fprintln(os.Stderr, "fake receiver:", err)
	os.Exit(99)
}

func main() {
	dir := os.Getenv("RECEIVER_CAPTURE_DIR")
	if dir == "" {
		fail(fmt.Errorf("RECEIVER_CAPTURE_DIR is required"))
	}
	argv, err := json.Marshal(os.Args[1:])
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "argv.json"), argv, 0o600); err != nil {
		fail(err)
	}
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stdin.bin"), stdin, 0o600); err != nil {
		fail(err)
	}
	env := append([]string{}, os.Environ()...)
	sort.Strings(env)
	rawEnv, err := json.Marshal(env)
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "env.json"), rawEnv, 0o600); err != nil {
		fail(err)
	}
	fd3 := os.NewFile(uintptr(3), "terminal")
	if fd3 == nil {
		fail(fmt.Errorf("fd3 is not open"))
	}
	fd3bytes, err := io.ReadAll(fd3)
	if err != nil {
		fail(fmt.Errorf("read fd3: %w", err))
	}
	if err := os.WriteFile(filepath.Join(dir, "fd3.txt"), fd3bytes, 0o600); err != nil {
		fail(err)
	}
	fmt.Println("receiver stdout is live")
	fmt.Fprintln(os.Stderr, "receiver stderr is live")
	status := os.NewFile(uintptr(4), "status")
	if status == nil {
		fail(fmt.Errorf("fd4 is not open"))
	}
	// Records replay first; an optional corrupt suffix follows, so mixed
	// shapes (complete frame then a transport fault) are expressible.
	if path := os.Getenv("RECEIVER_RECORDS"); path != "" {
		f, err := os.Open(path)
		if err != nil {
			fail(err)
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 128<<10), 128<<10)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var hdr [4]byte
			binary.BigEndian.PutUint32(hdr[:], uint32(len(line)))
			if _, err := status.Write(hdr[:]); err != nil {
				fail(err)
			}
			if _, err := status.Write(line); err != nil {
				fail(err)
			}
		}
		if err := scanner.Err(); err != nil {
			fail(err)
		}
		f.Close()
	}
	switch os.Getenv("RECEIVER_CORRUPT") {
	case "":
	case "zero":
		var hdr [4]byte
		_, _ = status.Write(hdr[:])
	case "garbage":
		_, _ = status.Write([]byte("not-framed"))
	case "oversize":
		var hdr [4]byte
		binary.BigEndian.PutUint32(hdr[:], 64<<10+1)
		_, _ = status.Write(hdr[:])
	case "truncated":
		var hdr [4]byte
		binary.BigEndian.PutUint32(hdr[:], 100)
		_, _ = status.Write(hdr[:])
		_, _ = status.Write([]byte("short"))
	case "badjson":
		body := []byte("{oops")
		var hdr [4]byte
		binary.BigEndian.PutUint32(hdr[:], uint32(len(body)))
		_, _ = status.Write(hdr[:])
		_, _ = status.Write(body)
	case "partial-header":
		_, _ = status.Write([]byte{0x00, 0x00})
	case "partial-body":
		var hdr [4]byte
		binary.BigEndian.PutUint32(hdr[:], 16)
		_, _ = status.Write(hdr[:])
		_, _ = status.Write([]byte("{}"))
	default:
		fail(fmt.Errorf("unknown RECEIVER_CORRUPT mode"))
	}
	status.Close()
	code := 0
	if raw := os.Getenv("RECEIVER_EXIT"); raw != "" {
		code, err = strconv.Atoi(raw)
		if err != nil {
			fail(err)
		}
	}
	os.Exit(code)
}
