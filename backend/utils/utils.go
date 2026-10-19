package utils

import (
	"bytes"
	"encoding/json"
	"os"
	"sync"
)

var fileLocks sync.Map

func GetLock(filename string) *sync.Mutex {
	lock, _ := fileLocks.LoadOrStore(filename, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func WithFileLock(filename string, fn func() error) error {
	lock := GetLock(filename)
	lock.Lock()
	defer lock.Unlock()
	return fn()
}

// trimUTF8BOM 去掉文件开头的 UTF-8 BOM。
// Windows 编辑器（记事本、部分 VS Code 配置）保存 JSON 时会写入 BOM，
// 而 encoding/json 不接受它，会直接报 "invalid character 'ï' looking for
// beginning of value" —— 用户导入自己编辑过的 data.json 时很容易踩到。
func trimUTF8BOM(data []byte) []byte {
	return bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
}

func ReadJSONUnlocked(filename string, v interface{}) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	return json.Unmarshal(trimUTF8BOM(data), v)
}

func WriteJSONUnlocked(filename string, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return writeFileAtomicUnlocked(filename, data)
}

// writeFileAtomicUnlocked 以「临时文件 → fsync → rename」落盘。
// rename 保证读者永远看不到半截文件；fsync 保证断电/容器被 kill 后
// 已返回成功的写入不会变成空文件或 0 字节（NAS 上掉电并不罕见）。
func writeFileAtomicUnlocked(filename string, data []byte) error {
	tempFile := filename + ".tmp"
	f, err := os.OpenFile(tempFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tempFile)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tempFile)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tempFile)
		return err
	}
	return os.Rename(tempFile, filename)
}

func AtomicWriteFile(filename string, data []byte) error {
	lock := GetLock(filename)
	lock.Lock()
	defer lock.Unlock()

	return writeFileAtomicUnlocked(filename, data)
}

func ReadJSON(filename string, v interface{}) error {
	lock := GetLock(filename)
	lock.Lock()
	defer lock.Unlock()

	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	return json.Unmarshal(trimUTF8BOM(data), v)
}

func WriteJSON(filename string, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return AtomicWriteFile(filename, data)
}
