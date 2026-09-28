package configfile

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	accessAllowedObjectACEType         = 5
	accessAllowedCallbackACEType       = 9
	accessAllowedCallbackObjectACEType = 11
	aceObjectTypePresent               = 0x1
	aceInheritedObjectTypePresent      = 0x2
)

func openFile(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p,
		windows.GENERIC_READ|windows.READ_CONTROL,
		windows.FILE_SHARE_READ,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return nil, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		_ = windows.CloseHandle(h)
		return nil, err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		_ = windows.CloseHandle(h)
		return nil, errSymlink
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("configuration file is not a regular file")
	}
	return os.NewFile(uintptr(h), path), nil
}

func validateFile(path string, file *os.File, _ os.FileInfo, _ os.FileInfo) error {
	fileOwner, dacl, closeSecurity, err := securityForHandle(windows.Handle(file.Fd()))
	if err != nil {
		return err
	}
	defer closeSecurity()

	dirPath, err := windows.UTF16PtrFromString(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("cannot inspect configuration directory owner: %w", err)
	}
	dirHandle, err := windows.CreateFile(dirPath,
		windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return fmt.Errorf("cannot inspect configuration directory owner: %w", err)
	}
	defer windows.CloseHandle(dirHandle)
	dirOwner, _, closeDirectorySecurity, err := securityForHandle(dirHandle)
	if err != nil {
		return fmt.Errorf("cannot inspect configuration directory owner: %w", err)
	}
	defer closeDirectorySecurity()
	if !fileOwner.Equals(dirOwner) {
		return fmt.Errorf("configuration file is owned by a different identity than its configuration directory")
	}
	if err := rejectForeignWriteGrants(dacl, fileOwner); err != nil {
		return err
	}
	return nil
}

func securityForHandle(handle windows.Handle) (*windows.SID, *windows.ACL, func(), error) {
	sd, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("cannot inspect Windows configuration security descriptor: %w", err)
	}
	free := func() { _, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(sd))) }
	owner, _, err := sd.Owner()
	if err != nil {
		free()
		return nil, nil, nil, fmt.Errorf("cannot inspect Windows configuration owner: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		free()
		return nil, nil, nil, fmt.Errorf("Windows DACL is absent: %w", err)
	}
	if dacl == nil {
		free()
		return nil, nil, nil, fmt.Errorf("Windows DACL is null and grants unrestricted access")
	}
	return owner, dacl, free, nil
}

func rejectForeignWriteGrants(dacl *windows.ACL, owner *windows.SID) error {
	writeMask := windows.ACCESS_MASK(windows.GENERIC_WRITE | windows.GENERIC_ALL | windows.FILE_GENERIC_WRITE |
		windows.DELETE | windows.WRITE_DAC | windows.WRITE_OWNER)
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return fmt.Errorf("cannot inspect Windows DACL entry: %w", err)
		}
		typ := ace.Header.AceType
		if typ == windows.ACCESS_DENIED_ACE_TYPE || isNonGrantingACE(typ) {
			continue
		}
		var sidOffset uintptr
		switch typ {
		case windows.ACCESS_ALLOWED_ACE_TYPE, accessAllowedCallbackACEType:
			sidOffset = 8
		case accessAllowedObjectACEType, accessAllowedCallbackObjectACEType:
			if ace.Header.AceSize < 12 {
				return fmt.Errorf("malformed Windows object DACL entry")
			}
			flags := *(*uint32)(unsafe.Add(unsafe.Pointer(ace), 8))
			sidOffset = 12
			if flags&aceObjectTypePresent != 0 {
				sidOffset += 16
			}
			if flags&aceInheritedObjectTypePresent != 0 {
				sidOffset += 16
			}
		default:
			return fmt.Errorf("Windows DACL contains an unsupported access entry (type %d)", typ)
		}
		if uint64(sidOffset)+8 > uint64(ace.Header.AceSize) {
			return fmt.Errorf("malformed Windows DACL entry")
		}
		sid := (*windows.SID)(unsafe.Add(unsafe.Pointer(ace), sidOffset))
		if !sid.IsValid() || sidOffset+uintptr(sid.Len()) > uintptr(ace.Header.AceSize) {
			return fmt.Errorf("malformed Windows DACL identity")
		}
		if hasForeignWriteGrant(uint32(ace.Mask), uint32(writeMask), sid.String(), owner.String()) {
			return fmt.Errorf("Windows DACL grants write access to another identity")
		}
	}
	return nil
}

func isNonGrantingACE(typ uint8) bool {
	switch typ {
	case 2, 3, 6, 7, 8, 10, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21:
		return true
	default:
		return false
	}
}
