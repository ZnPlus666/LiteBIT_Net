package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Layout struct{ Unit, CodeDir, DataDir, UnitPath, CodePath, ConfigPath, KeyPath, LogPath string }

func makeLayout() (Layout, error) {
	l := Layout{Unit: "litebit-net-" + mode + ".service"}
	if mode == "link2user" {
		u, err := user.LookupId(strconv.Itoa(os.Getuid()))
		if err != nil || !filepath.IsAbs(u.HomeDir) {
			return l, fail("无法读取用户主目录。 / Cannot read the user's home directory.")
		}
		l.CodeDir = filepath.Join(u.HomeDir, ".local/share/LiteBIT_Net_linux_link2user")
		l.DataDir = l.CodeDir
		l.UnitPath = filepath.Join(u.HomeDir, ".config/systemd/user", l.Unit)
	} else {
		l.CodeDir = "/opt/LiteBIT_Net_linux_link2PC"
		l.DataDir = "/var/lib/LiteBIT_Net_linux_link2PC"
		l.UnitPath = filepath.Join("/etc/systemd/system", l.Unit)
	}
	l.CodePath = filepath.Join(l.CodeDir, "litebit_linux_"+mode)
	l.ConfigPath = filepath.Join(l.DataDir, "config.json")
	l.KeyPath = filepath.Join(l.DataDir, "credential.key")
	l.LogPath = filepath.Join(l.DataDir, "litebit-net.log")
	return l, nil
}
func toolPath(name string) (string, error) {
	for _, directory := range []string{"/usr/bin", "/bin"} {
		path := directory + "/" + name
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
			return path, nil
		}
	}
	return "", fail("未找到系统工具 / System tool not found: " + name)
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 65536 {
		return 0, fail("输出过长。 / Output too large.")
	}
	return b.Buffer.Write(p)
}
func runTool(ctx context.Context, name string, args ...string) (string, int, error) {
	path, err := toolPath(name)
	if err != nil {
		return "", -1, err
	}
	command := exec.CommandContext(ctx, path, args...)
	command.WaitDelay = time.Second
	// Pin the per-user manager to this UID, not a caller-supplied D-Bus socket.
	command.Env = os.Environ()
	if mode == "link2user" {
		filtered := []string{}
		for _, s := range command.Env {
			if !strings.HasPrefix(s, "XDG_RUNTIME_DIR=") && !strings.HasPrefix(s, "DBUS_SESSION_BUS_ADDRESS=") {
				filtered = append(filtered, s)
			}
		}
		runtimeDir := fmt.Sprintf("/run/user/%d", os.Getuid())
		command.Env = append(filtered, "XDG_RUNTIME_DIR="+runtimeDir, "DBUS_SESSION_BUS_ADDRESS=unix:path="+runtimeDir+"/bus")
	}
	var out, stderr limitedBuffer
	command.Stdout = &out
	command.Stderr = &stderr
	err = command.Run()
	code := 0
	if err != nil {
		code = -1
		if e, ok := err.(*exec.ExitError); ok {
			code = e.ExitCode()
		}
	}
	return out.String(), code, err
}
func properties(s string) map[string]string {
	result := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			result[key] = value
		}
	}
	return result
}
func sessionMatches(uid string, p map[string]string) bool {
	class := p["Class"]
	return p["User"] == uid && (class == "user" || class == "user-early" || class == "user-light" || class == "user-early-light") && (p["State"] == "active" || p["State"] == "online")
}
func hasLoginSession(ctx context.Context) bool {
	uid := strconv.Itoa(os.Getuid())
	output, code, err := runTool(ctx, "loginctl", "show-user", uid, "-p", "Sessions", "--value")
	if err != nil || code != 0 {
		return false
	}
	for _, session := range strings.Fields(output) {
		if ctx.Err() != nil {
			return false
		}
		if !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(session) {
			continue
		}
		output, code, err = runTool(ctx, "loginctl", "show-session", session, "-p", "User", "-p", "Class", "-p", "State")
		p := properties(output)
		if err == nil && code == 0 && sessionMatches(uid, p) {
			return true
		}
	}
	return false
}
func preflight(ctx context.Context) error {
	if runtime.GOOS != "linux" {
		return fail("此版本仅适用于 Linux。 / This version requires Linux.")
	}
	if mode != "link2user" && mode != "link2PC" {
		return fail("程序模式无效。 / Invalid program mode.")
	}
	if mode == "link2PC" && os.Geteuid() != 0 {
		return fail("需要管理员权限，请使用 sudo。 / Administrator privileges required; use sudo.")
	}
	if mode == "link2user" && os.Geteuid() == 0 {
		return fail("关联用户版请使用普通用户运行，不要使用 sudo。 / Run link2user as a regular user, without sudo.")
	}
	if info, err := os.Stat("/run/systemd/system"); err != nil || !info.IsDir() {
		return fail("需要运行中的 systemd。 / A running systemd installation is required.")
	}
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, code, err := runTool(check, "systemctl", "--version")
	match := regexp.MustCompile(`^systemd\s+(\d+)`).FindStringSubmatch(output)
	if err != nil || code != 0 || match == nil {
		return fail("无法读取 systemd 版本。 / Cannot read the systemd version.")
	}
	version, _ := strconv.Atoi(match[1])
	if version < 245 {
		return fail("需要 systemd 245 或更新版本。 / systemd 245 or later is required.")
	}
	if mode == "link2user" {
		if _, err := toolPath("loginctl"); err != nil {
			return fail("需要 systemd-logind 和 loginctl。 / systemd-logind and loginctl are required.")
		}
	}
	return nil
}

type TaskManager interface {
	State(context.Context) (map[string]string, error)
	Run(context.Context, ...string) error
	WaitActive(context.Context) error
}
type Manager struct{ Layout Layout }

func (m Manager) command(ctx context.Context, args ...string) (string, int, error) {
	check, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if mode == "link2user" {
		args = append([]string{"--user"}, args...)
	}
	return runTool(check, "systemctl", args...)
}
func (m Manager) Run(ctx context.Context, args ...string) error {
	_, code, err := m.command(ctx, args...)
	if err != nil || code != 0 {
		return fail("服务操作失败，请检查 systemd 权限和登录会话。 / Service operation failed; check systemd permissions and the login session.")
	}
	return nil
}
func (m Manager) State(ctx context.Context) (map[string]string, error) {
	output, code, _ := m.command(ctx, "show", m.Layout.Unit, "-p", "LoadState", "-p", "ActiveState", "-p", "UnitFileState", "-p", "FragmentPath")
	p := properties(output)
	if p["LoadState"] == "" || (code != 0 && p["LoadState"] != "not-found") {
		return nil, fail("无法读取服务状态。 / Cannot read service status.")
	}
	return p, nil
}
func (m Manager) WaitActive(ctx context.Context) error {
	for i := 0; i < 8; i++ {
		if err := sleepContext(ctx, time.Second); err != nil {
			return err
		}
		p, err := m.State(ctx)
		if err != nil {
			return err
		}
		if p["ActiveState"] == "active" {
			return nil
		}
	}
	return fail("任务未运行。 / The task is not running.")
}
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return fail("操作已取消。 / Operation cancelled.")
	}
}
func marker() string { return "# LiteBIT_Net managed service: " + mode }
func unitQuote(path string) (string, error) {
	for _, r := range path {
		if r < 32 || r == 127 {
			return "", fail("程序路径无效。 / Invalid program path.")
		}
	}
	r := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "%", "%%", "$", "$$")
	return "\"" + r.Replace(path) + "\"", nil
}
func serviceText(l Layout) ([]byte, error) {
	code, err := unitQuote(l.CodePath)
	if err != nil {
		return nil, err
	}
	data, err := unitQuote(l.DataDir)
	if err != nil {
		return nil, err
	}
	network, hardening, target := "", "NoNewPrivileges=true\n", "default.target"
	if mode == "link2PC" {
		network = "Wants=network-online.target\nAfter=network-online.target\n"
		hardening += "PrivateTmp=true\nProtectSystem=strict\nProtectHome=true\nReadWritePaths=" + data + "\n"
		target = "multi-user.target"
	}
	return []byte(marker() + "\n[Unit]\nDescription=LiteBIT_Net Linux " + mode + "\n" + network + "\n[Service]\nType=simple\nExecStart=" + code + " monitor\nRestart=on-failure\nRestartSec=30\nUMask=0077\n" + hardening + "\n[Install]\nWantedBy=" + target + "\n"), nil
}
func requireOwnedUnit(l Layout, p map[string]string) error {
	if fragment := p["FragmentPath"]; fragment != "" && filepath.Clean(fragment) != filepath.Clean(l.UnitPath) {
		return fail("同名服务来自其他位置，未修改。 / A service with this name exists elsewhere; no changes made.")
	}
	body, err := readManaged(l.UnitPath, false, 262144)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !bytes.HasPrefix(body, []byte(marker()+"\n")) {
		return fail("同名服务不是本工具创建的，未修改。 / The existing service was not created by this tool; no changes made.")
	}
	return nil
}
