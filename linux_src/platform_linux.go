//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

func ownerUID(info os.FileInfo) int { return int(info.Sys().(*syscall.Stat_t).Uid) }
func openNoFollow(path string, flags int, perm os.FileMode) (*os.File, error) {
	fd, err := syscall.Open(path, flags|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, uint32(perm.Perm()))
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
func privateUmask() { syscall.Umask(0077) }
func terminalState() (*syscall.Termios, error) {
	var state syscall.Termios
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, os.Stdin.Fd(), syscall.TCGETS, uintptr(unsafe.Pointer(&state)))
	if err != 0 {
		return nil, fail("请在交互终端中安装。 / Install from an interactive terminal.")
	}
	return &state, nil
}
func hideEcho() (func(), error) {
	old, err := terminalState()
	if err != nil {
		return nil, err
	}
	next := *old
	next.Lflag &^= syscall.ECHO | syscall.ECHONL
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, os.Stdin.Fd(), syscall.TCSETS, uintptr(unsafe.Pointer(&next)))
	if e != 0 {
		return nil, fail("无法读取密码。 / Cannot read the password.")
	}
	return func() {
		_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, os.Stdin.Fd(), syscall.TCSETS, uintptr(unsafe.Pointer(old)))
	}, nil
}
func acquireOperationLock() (func(), error) {
	directory := fmt.Sprintf("/run/user/%d", os.Getuid())
	if mode == "link2PC" {
		directory = "/run/LiteBIT_Net_linux_link2PC"
		if err := secureDirectory(directory); err != nil {
			return nil, err
		}
	}
	if err := plainPath(directory); err != nil {
		return nil, err
	}
	f, err := openNoFollow(filepath.Join(directory, "litebit-net-"+mode+".lock"), os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, fail("无法创建锁文件。 / Cannot create the lock file.")
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || ownerUID(info) != os.Geteuid() || info.Mode().Perm()&0077 != 0 {
		f.Close()
		return nil, fail("锁文件权限无效。 / Invalid lock-file permissions.")
	}
	if syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		f.Close()
		return nil, fail("另一个安装或卸载正在进行。 / Another installation or uninstall is in progress.")
	}
	return func() { _ = f.Close() }, nil
}
