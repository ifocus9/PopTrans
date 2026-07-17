package logging

import (
	"io"
	"log"
	"os"
	"sync"
)

var (
	mu          sync.Mutex
	currentFile *os.File
)

// Configure switches the process-wide standard logger between a file and a
// discard sink. Disabled logging never creates or touches the log file.
func Configure(path string, enabled bool) error {
	mu.Lock()
	defer mu.Unlock()

	if currentFile != nil {
		_ = currentFile.Close()
		currentFile = nil
	}

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	if !enabled {
		log.SetOutput(io.Discard)
		return nil
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.SetOutput(io.Discard)
		return err
	}

	currentFile = file
	log.SetOutput(file)
	return nil
}

func Close() {
	mu.Lock()
	defer mu.Unlock()

	if currentFile != nil {
		_ = currentFile.Close()
		currentFile = nil
	}
	log.SetOutput(io.Discard)
}
