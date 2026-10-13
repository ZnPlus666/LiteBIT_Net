package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func plainPath(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fail("安装路径无效。 / Invalid installation path.")
	}
	for part := abs; ; part = filepath.Dir(part) {
		info, err := os.Lstat(part)
		if err != nil && !os.IsNotExist(err) {
			return fail("无法检查安装路径。 / Cannot inspect the installation path.")
		}
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fail("拒绝使用链接路径。 / Refusing a symbolic-link path.")
			}
			if part != abs || info.IsDir() {
				if !info.IsDir() {
					return fail("安装路径不是目录。 / Installation path is not a directory.")
				}
				stickyRoot := ownerUID(info) == 0 && info.Mode()&os.ModeSticky != 0
				if (ownerUID(info) != 0 && ownerUID(info) != os.Geteuid()) || (info.Mode().Perm()&0022 != 0 && !stickyRoot) {
					return fail("目录所有者或权限不安全。 / Unsafe directory ownership or permissions.")
				}
			}
		}
		if part == filepath.Dir(part) {
			break
		}
	}
	return nil
}
func secureDirectory(path string) error {
	if err := plainPath(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return fail("无法创建目录。 / Cannot create the directory.")
	}
	info, err := os.Lstat(path)
	if err != nil || ownerUID(info) != os.Geteuid() {
		return fail("目录不属于当前执行账户。 / Directory is not owned by the current account.")
	}
	if os.Chmod(path, 0700) != nil {
		return fail("无法设置目录权限。 / Cannot set directory permissions.")
	}
	return nil
}
func readManaged(path string, private bool, limit int64) ([]byte, error) {
	if err := plainPath(path); err != nil {
		return nil, err
	}
	f, err := openNoFollow(path, os.O_RDONLY, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, err
		}
		return nil, fail("无法读取文件。 / Cannot read the file.")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || ownerUID(info) != os.Geteuid() || info.Mode().Perm()&0022 != 0 || (private && info.Mode().Perm()&0077 != 0) {
		return nil, fail("文件所有者或权限不安全。 / Unsafe file ownership or permissions.")
	}
	body, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, fail("文件大小无效或无法读取。 / Invalid file size or unreadable file.")
	}
	return body, nil
}
func atomicWrite(path string, body []byte, perm os.FileMode) error {
	if err := plainPath(path); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".litebit-")
	if err != nil {
		return fail("无法创建临时文件。 / Cannot create a temporary file.")
	}
	temp := f.Name()
	defer os.Remove(temp)
	if err = f.Chmod(perm); err == nil {
		_, err = f.Write(body)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return fail("无法保存文件。 / Cannot save the file.")
	}
	if err = plainPath(path); err != nil {
		return err
	}
	if os.Rename(temp, path) != nil {
		return fail("无法替换文件。 / Cannot replace the file.")
	}
	return nil
}
func removeManaged(path string, private bool, limit int64) error {
	if _, err := readManaged(path, private, limit); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if os.Remove(path) != nil {
		return fail("无法删除文件。 / Cannot remove the file.")
	}
	return nil
}
func loadConfig(path string) (Config, error) {
	body, err := readManaged(path, true, 262144)
	if err != nil {
		return Config{}, err
	}
	var c Config
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&c) != nil {
		return c, fail("配置无效；旧 Python 版请重新安装。 / Invalid configuration; reinstall an older Python version.")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return c, fail("配置无效。 / Invalid configuration.")
	}
	return c, validateConfig(c)
}

type FileLog struct {
	Path    string
	Console bool
}

func (l FileLog) Emit(message string) error {
	body, err := readManaged(l.Path, true, 2*1024*1024)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(body) > 1024*1024 {
		if err := removeManaged(l.Path+".2", true, 2*1024*1024); err != nil {
			return err
		}
		if _, err := readManaged(l.Path+".1", true, 2*1024*1024); err == nil {
			if os.Rename(l.Path+".1", l.Path+".2") != nil {
				return fail("日志轮转失败。 / Log rotation failed.")
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := plainPath(l.Path + ".1"); err != nil {
			return err
		}
		if os.Rename(l.Path, l.Path+".1") != nil {
			return fail("日志轮转失败。 / Log rotation failed.")
		}
	}
	if err := plainPath(l.Path); err != nil {
		return err
	}
	f, err := openNoFollow(l.Path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return fail("无法写入日志。 / Cannot write the log.")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || ownerUID(info) != os.Geteuid() || info.Mode().Perm()&0077 != 0 {
		return fail("日志权限无效。 / Invalid log permissions.")
	}
	line := time.Now().Format("2006-01-02 15:04:05 ") + message
	if _, err = fmt.Fprintln(f, line); err != nil {
		return fail("无法写入日志。 / Cannot write the log.")
	}
	if l.Console {
		fmt.Println(line)
	}
	return nil
}
