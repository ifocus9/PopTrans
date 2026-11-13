package windows

import (
	"syscall"
	"unsafe"
)

const errorAlreadyExists = syscall.Errno(183)

var (
	procCreateMutexW = modKernel32.NewProc("CreateMutexW")
	singleInstance   uintptr
)

// AcquireSingleInstance creates a per-session named mutex for the host process.
// alreadyRunning reports that another host owns the mutex.
func AcquireSingleInstance(name string) (alreadyRunning bool, err error) {
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return false, err
	}
	handle, _, callErr := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(namePtr)))
	if handle == 0 {
		return false, callErr
	}
	if callErr == errorAlreadyExists {
		procCloseHandle.Call(handle)
		return true, nil
	}
	singleInstance = handle
	return false, nil
}

// ReleaseSingleInstance closes the mutex handle held by the current process.
func ReleaseSingleInstance() {
	if singleInstance != 0 {
		procCloseHandle.Call(singleInstance)
		singleInstance = 0
	}
}
