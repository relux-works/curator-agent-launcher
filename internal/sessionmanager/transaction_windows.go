package sessionmanager

import (
	"bufio"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

func openRegistryLock(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_ALWAYS, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	var info windows.ByHandleFileInformation
	err = windows.GetFileInformationByHandle(h, &info)
	if err != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		windows.CloseHandle(h)
		if err == nil {
			err = os.ErrInvalid
		}
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}
func tryRegistryLock(f *os.File) (bool, error) {
	err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
	if err == windows.ERROR_LOCK_VIOLATION {
		return false, nil
	}
	return err == nil, err
}
func readPromptFile(f *os.File, timeout time.Duration) (string, error) {
	// Refuse before writing if this handle cannot provide a bounded read.
	if err := f.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return "", err
	}
	defer f.SetReadDeadline(time.Time{})
	return bufio.NewReader(f).ReadString('\n')
}
