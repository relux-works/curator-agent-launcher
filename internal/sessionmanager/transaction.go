package sessionmanager

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"time"
)

const registryLockTimeout = 10 * time.Second
const promptAnswerTimeout = 120 * time.Second

// Never unlink the lock: every contender must open the same inode, including
// after a crashed holder. Closing the handle releases the OS advisory lock.
func acquireRegistryLock(path string, mode os.FileMode) (*os.File, error) {
	dirMode := os.FileMode(0700)
	if mode == 0644 {
		dirMode = 0755
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return nil, invalid(path, "unreadable", err)
	}
	lockPath := path + ".lock"
	f, err := openRegistryLock(lockPath)
	if err != nil {
		return nil, invalid(lockPath, "unreadable", err)
	}
	deadline := time.Now().Add(registryLockTimeout)
	for {
		locked, err := tryRegistryLock(f)
		if err != nil {
			f.Close()
			return nil, invalid(lockPath, "unreadable", err)
		}
		if locked {
			return f, nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			f.Close()
			return nil, &Error{Code: CodeBusy}
		}
		time.Sleep(min(25*time.Millisecond, remaining))
	}
}

func readPrompt(in io.Reader, timeout time.Duration) (string, error) {
	if file, ok := in.(*os.File); ok {
		return readPromptFile(file, timeout)
	}
	// Non-file readers are synchronous Service inputs (for example, an in-memory
	// answer). Real terminal input always takes the deadline-aware OS path above.
	return bufio.NewReader(in).ReadString('\n')
}
