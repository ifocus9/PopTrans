package windows

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

const (
	th32csSnapProcess              = 0x00000002
	processTerminate               = 0x0001
	processQueryLimitedInformation = 0x1000
	processImagePathMax            = 32 * 1024
)

var (
	procCreateToolhelp32Snapshot   = modKernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW            = modKernel32.NewProc("Process32FirstW")
	procProcess32NextW             = modKernel32.NewProc("Process32NextW")
	procOpenProcess                = modKernel32.NewProc("OpenProcess")
	procQueryFullProcessImageNameW = modKernel32.NewProc("QueryFullProcessImageNameW")
	procTerminateProcess           = modKernel32.NewProc("TerminateProcess")
	procCloseHandle                = modKernel32.NewProc("CloseHandle")
)

type processEntry32 struct {
	size            uint32
	cntUsage        uint32
	processID       uint32
	defaultHeapID   uintptr
	moduleID        uint32
	cntThreads      uint32
	parentProcessID uint32
	priClassBase    int32
	flags           uint32
	exeFile         [syscall.MAX_PATH]uint16
}

// KillProcessesByImagePath terminates every process whose full executable path
// matches target (case-insensitive), including PyInstaller's bootloader child.
// It uses the Toolhelp snapshot instead of a shell command, so the path is
// never interpreted as script text.
func KillProcessesByImagePath(target string) (int, error) {
	cleaned := filepath.Clean(target)
	if cleaned == "" || cleaned == "." {
		return 0, fmt.Errorf("invalid process image path: %q", target)
	}

	snapshot, _, callErr := procCreateToolhelp32Snapshot.Call(th32csSnapProcess, 0)
	if snapshot == uintptr(syscall.InvalidHandle) {
		return 0, fmt.Errorf("create process snapshot: %w", callErr)
	}
	defer procCloseHandle.Call(snapshot)

	self := uint32(os.Getpid())
	entry := processEntry32{size: uint32(unsafe.Sizeof(processEntry32{}))}
	ok, _, _ := procProcess32FirstW.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	killed := 0
	for ok != 0 {
		pid := entry.processID
		if pid != 0 && pid != self {
			if imagePath, err := processImagePath(pid); err == nil && strings.EqualFold(imagePath, cleaned) {
				if err := terminateProcessByPID(pid); err == nil {
					killed++
				}
			}
		}
		entry.size = uint32(unsafe.Sizeof(processEntry32{}))
		ok, _, _ = procProcess32NextW.Call(snapshot, uintptr(unsafe.Pointer(&entry)))
	}
	return killed, nil
}

func processImagePath(pid uint32) (string, error) {
	handle, _, callErr := procOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if handle == 0 {
		return "", callErr
	}
	defer procCloseHandle.Call(handle)

	buffer := make([]uint16, processImagePathMax)
	size := uint32(len(buffer))
	ok, _, callErr := procQueryFullProcessImageNameW.Call(
		handle,
		0,
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if ok == 0 {
		return "", callErr
	}
	return syscall.UTF16ToString(buffer[:size]), nil
}

func terminateProcessByPID(pid uint32) error {
	handle, _, callErr := procOpenProcess.Call(processTerminate, 0, uintptr(pid))
	if handle == 0 {
		return callErr
	}
	defer procCloseHandle.Call(handle)
	ok, _, callErr := procTerminateProcess.Call(handle, 1)
	if ok == 0 {
		return callErr
	}
	return nil
}
