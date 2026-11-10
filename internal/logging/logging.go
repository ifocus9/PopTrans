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

// MaxLogBytes 单个日志文件上限，超过则轮转为 translate.log.1（仅保留一份，
// 再次轮转时覆盖旧的 .1）。translate.log 同时被宿主进程和 AI 引擎
// 以追加方式使用，且没有轮转逻辑时会随使用时长无限增长。
const MaxLogBytes = 16 << 20 // 16MB

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

	rotateIfLarge(path, MaxLogBytes)
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

// rotateIfLarge 在写日志前把超限文件改名保留一份历史。
// 仅在进程启动/启用日志时触发一次，不在写路径上引入锁竞争。
func rotateIfLarge(path string, maxBytes int64) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < maxBytes {
		return
	}
	rotated := path + ".1"
	_ = os.Remove(rotated)
	// 改名失败也不阻塞日志（下次启动会重试）；此时日志器尚未指向文件，打印无意义。
	_ = os.Rename(path, rotated)
}
