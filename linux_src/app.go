package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Each release mode is built from this same source via -X main.mode=...
var mode = "link2user"
var buildVersion = "native-v2"

type controlled struct{ Message string }

type Store interface {
	Prepare(Layout) error
	Read(string, bool, int64) ([]byte, error)
	Write(string, []byte, os.FileMode) error
	Remove(string, bool, int64) error
}
type diskStore struct{}

func (diskStore) Prepare(l Layout) error {
	for _, dir := range []string{l.CodeDir, l.DataDir} {
		if err := secureDirectory(dir); err != nil {
			return err
		}
	}
	parent := filepath.Dir(l.UnitPath)
	if err := plainPath(parent); err != nil {
		return err
	}
	if os.MkdirAll(parent, 0700) != nil {
		return fail("无法创建服务目录。 / Cannot create the service directory.")
	}
	return nil
}
func (diskStore) Read(p string, private bool, limit int64) ([]byte, error) {
	return readManaged(p, private, limit)
}
func (diskStore) Write(p string, b []byte, perm os.FileMode) error { return atomicWrite(p, b, perm) }
func (diskStore) Remove(p string, private bool, limit int64) error {
	return removeManaged(p, private, limit)
}

type Input struct {
	Reader  *bufio.Reader
	Context context.Context
}

func (in *Input) line(prompt string) (string, error) {
	fmt.Print(prompt)
	type result struct {
		s   string
		err error
	}
	channel := make(chan result, 1)
	go func() { s, err := in.Reader.ReadString('\n'); channel <- result{s, err} }()
	select {
	case <-in.Context.Done():
		return "", fail("操作已取消。 / Operation cancelled.")
	case r := <-channel:
		if r.err != nil {
			return "", fail("操作已取消。 / Operation cancelled.")
		}
		if len(r.s) > 4096 {
			return "", fail("输入过长。 / Input too long.")
		}
		return strings.TrimSuffix(strings.TrimSuffix(r.s, "\n"), "\r"), nil
	}
}
func (in *Input) password(prompt string) (string, error) {
	restore, err := hideEcho()
	if err != nil {
		return "", err
	}
	defer restore()
	defer fmt.Println()
	return in.line(prompt)
}
func parseInterval(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 600, nil
	}
	if !regexp.MustCompile(`^[0-9]{1,8}$`).MatchString(value) {
		return 0, fail("请输入有效的正整数分钟数。 / Enter a valid positive whole number of minutes.")
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 1 || n > maxIntervalMinutes {
		return 0, fail("请输入有效的正整数分钟数。 / Enter a valid positive whole number of minutes.")
	}
	return n * 60, nil
}
func readCredentials(in *Input) (Config, string, error) {
	fmt.Println("请输入校园网账号和密码。 / Enter your campus-network account and password.")
	name, err := in.line("校园网账号 / Campus-network account: ")
	if err != nil {
		return Config{}, "", err
	}
	pw, err := in.password("校园网密码 / Campus-network password: ")
	if err != nil {
		return Config{}, "", err
	}
	confirm, err := in.password("再次输入密码 / Repeat password: ")
	if err != nil {
		return Config{}, "", err
	}
	if pw != confirm {
		return Config{}, "", fail("两次输入密码不一致。 / The passwords do not match.")
	}
	name = strings.TrimSpace(name)
	if err = validateCredentials(name, pw); err != nil {
		return Config{}, "", err
	}
	var interval int64
	for {
		value, e := in.line("检查间隔（分钟，默认10） / Check interval (minutes, default 10): ")
		if e != nil {
			return Config{}, "", e
		}
		interval, err = parseInterval(value)
		if err == nil {
			break
		}
		fmt.Println(err)
	}
	c := Config{Format: 2, Mode: mode, Server: "http://10.0.0.55", Username: name, AcID: "8", Interval: interval}
	return c, pw, validateConfig(c)
}
func setupVerification(p *Portal, read func(string) (string, error)) error {
	fmt.Println("正在验证新凭据…… / Verifying new credentials...")
	result := p.verify(false)
	if result == "DisconnectRequired" {
		answer, err := read("进行账号密码正确性验证将注销当前登录！输入 Y 确认验证，输入 N 或其他跳过验证。 / Verifying the account and password will sign out of the current session! Enter Y to verify, or N or anything else to skip verification. ")
		if err != nil {
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(answer), "Y") {
			p.emit("VERIFY_SKIPPED 已跳过账号密码正确性验证。 / Account/password verification skipped.")
			return nil
		}
		result = p.verify(true)
	}
	if result == "Unknown" {
		answer, err := read("是否继续创建任务？输入 Y 确认，输入 N 或其他中止。 / Continue creating the task? Enter Y to confirm, or N or anything else to abort. ")
		if err != nil {
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(answer), "Y") {
			return fail("安装已取消。 / Installation cancelled.")
		}
		p.emit("VERIFY_CONTINUED 验证结果未知，继续安装。 / Verification result unknown; continuing installation.")
		return nil
	}
	if result != "Verified" {
		return fail("新凭据验证失败，未保存新配置。 / New credential verification failed; new configuration was not saved.")
	}
	return nil
}
func ownedUnit(l Layout, state map[string]string, store Store) error {
	if fragment := state["FragmentPath"]; fragment != "" && filepath.Clean(fragment) != filepath.Clean(l.UnitPath) {
		return fail("同名服务来自其他位置，未修改。 / A service with this name exists elsewhere; no changes made.")
	}
	body, err := store.Read(l.UnitPath, false, 262144)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !strings.HasPrefix(string(body), marker()+"\n") {
		return fail("同名服务不是本工具创建的，未修改。 / The existing service was not created by this tool; no changes made.")
	}
	return nil
}
func isRunning(state string) bool {
	return state == "active" || state == "activating" || state == "reloading"
}
func installConfig(ctx context.Context, l Layout, m TaskManager, store Store, c Config, password string, source []byte, verify func() error) (result error) {
	if err := validateConfig(c); err != nil {
		return err
	}
	if err := validateCredentials(c.Username, password); err != nil {
		return err
	}
	state, err := m.State(ctx)
	if err != nil {
		return err
	}
	if err = ownedUnit(l, state, store); err != nil {
		return err
	}
	if state["LoadState"] == "masked" {
		return fail("任务已被屏蔽，请先解除屏蔽。 / The task is masked; unmask it first.")
	}
	if err = store.Prepare(l); err != nil {
		return err
	}
	paths := []string{l.ConfigPath, l.KeyPath, l.CodePath, l.UnitPath}
	previous := map[string][]byte{}
	for _, path := range paths {
		limit := int64(262144)
		if path == l.CodePath {
			limit = 32 * 1024 * 1024
		}
		body, e := store.Read(path, path != l.UnitPath, limit)
		if e != nil && !os.IsNotExist(e) {
			return e
		}
		previous[path] = body
	}
	key := previous[l.KeyPath]
	if key == nil {
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return fail("无法生成密钥。 / Cannot generate the credential key.")
		}
	}
	if len(key) != 32 {
		return fail("密钥文件无效。 / Invalid key file.")
	}
	sealed, err := sealConfig(c, password, key)
	if err != nil {
		return err
	}
	body, err := json.MarshalIndent(sealed, "", "  ")
	if err != nil {
		return fail("配置无效。 / Invalid configuration.")
	}
	body = append(body, '\n')
	unit, err := serviceText(l)
	if err != nil {
		return err
	}
	running := isRunning(state["ActiveState"])
	changed, completed := false, false
	defer func() {
		if completed {
			return
		}
		// Rollback must still run after Ctrl+C cancels the operation context.
		recovery, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		restored := true
		if changed {
			// Do not overwrite a binary/configuration while its new monitor still runs.
			if e := m.Run(recovery, "stop", l.Unit); e != nil {
				restored = false
			}
			p, e := m.State(recovery)
			if e != nil || isRunning(p["ActiveState"]) || p["ActiveState"] == "deactivating" {
				restored = false
			}
			if restored {
				if m.Run(recovery, "disable", l.Unit) != nil {
					restored = false
				}
				for _, path := range paths {
					perm := os.FileMode(0600)
					limit := int64(262144)
					if path == l.UnitPath {
						perm = 0644
					}
					if path == l.CodePath {
						perm = 0700
						limit = 32 * 1024 * 1024
					}
					var e error
					if previous[path] == nil {
						e = store.Remove(path, path != l.UnitPath, limit)
					} else {
						e = store.Write(path, previous[path], perm)
					}
					if e != nil {
						restored = false
					}
				}
				if m.Run(recovery, "daemon-reload") != nil {
					restored = false
				}
				if state["UnitFileState"] == "enabled" || state["UnitFileState"] == "enabled-runtime" {
					args := []string{"enable", l.Unit}
					if state["UnitFileState"] == "enabled-runtime" {
						args = []string{"enable", "--runtime", l.Unit}
					}
					if m.Run(recovery, args...) != nil {
						restored = false
					}
				}
			}
			if !restored {
				fmt.Println("旧配置或任务恢复失败。 / Previous configuration or task restoration failed.")
			}
		}
		if running {
			fmt.Println("正在恢复原任务…… / Restoring previous task...")
			if restored && m.Run(recovery, "start", l.Unit) == nil && m.WaitActive(recovery) == nil {
				fmt.Println("原任务已恢复。 / Previous task restored.")
			} else {
				fmt.Println("原任务恢复失败。 / Previous task restoration failed.")
			}
		}
	}()
	if running {
		if err = m.Run(ctx, "stop", l.Unit); err != nil {
			return err
		}
		p, e := m.State(ctx)
		if e != nil {
			return e
		}
		if isRunning(p["ActiveState"]) || p["ActiveState"] == "deactivating" {
			return fail("无法停止原任务，未验证新凭据。 / Cannot stop the previous task; new credentials were not verified.")
		}
	}
	if err = verify(); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return fail("操作已取消。 / Operation cancelled.")
	}
	changed = true
	for _, file := range []struct {
		path string
		body []byte
		perm os.FileMode
	}{{l.CodePath, source, 0700}, {l.KeyPath, key, 0600}, {l.ConfigPath, body, 0600}, {l.UnitPath, unit, 0644}} {
		if err = store.Write(file.path, file.body, file.perm); err != nil {
			return err
		}
	}
	fmt.Println("配置已保存。 / Configuration saved.")
	if err = m.Run(ctx, "daemon-reload"); err != nil {
		return err
	}
	if err = m.Run(ctx, "enable", l.Unit); err != nil {
		return err
	}
	fmt.Println("正在启动任务…… / Starting task...")
	if err = m.Run(ctx, "start", l.Unit); err != nil {
		return err
	}
	if err = m.WaitActive(ctx); err != nil {
		return err
	}
	completed = true
	fmt.Println("安装完成。 / Installed.")
	fmt.Println("服务 / Service: " + l.Unit)
	fmt.Println("程序目录 / Program directory: " + l.CodeDir)
	fmt.Println("日志文件 / Log file: " + l.LogPath)
	return nil
}
func install(ctx context.Context, l Layout, m Manager) error {
	if _, err := terminalState(); err != nil {
		return err
	}
	if mode == "link2user" {
		check, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_, code, _ := runTool(check, "systemctl", "is-active", "--quiet", "litebit-net-link2PC.service")
		if code == 0 {
			return fail("请先卸载关联电脑版，再安装关联用户版。 / Uninstall link2PC before installing link2user.")
		}
		if !hasLoginSession(check) {
			return fail("请在当前用户的正常登录会话中安装。 / Install from a normal login session of the current user.")
		}
	}
	in := &Input{bufio.NewReader(io.LimitReader(os.Stdin, 1024*1024)), ctx}
	c, password, err := readCredentials(in)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return fail("无法读取程序路径。 / Cannot read the program path.")
	}
	f, err := os.Open(exe)
	if err != nil {
		return fail("无法读取程序。 / Cannot read the program.")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 32*1024*1024 {
		return fail("程序文件无效。 / Invalid program file.")
	}
	source, err := io.ReadAll(io.LimitReader(f, 32*1024*1024+1))
	if err != nil || len(source) > 32*1024*1024 {
		return fail("无法读取程序。 / Cannot read the program.")
	}
	p := newPortal(c, password, FileLog{l.LogPath, true}, ctx)
	defer func() { p.Password = ""; password = "" }()
	return installConfig(ctx, l, m, diskStore{}, c, password, source, func() error { return setupVerification(p, in.line) })
}
func showStatus(ctx context.Context, l Layout, m TaskManager) error {
	p, err := m.State(ctx)
	if err != nil {
		return err
	}
	fmt.Println("服务 / Service: " + l.Unit)
	fmt.Println("运行状态 / State: " + safeProperty(p["ActiveState"]))
	fmt.Println("开机或登录启动 / Enabled: " + safeProperty(p["UnitFileState"]))
	if p["LoadState"] == "not-found" {
		fmt.Println("未找到任务。 / Task not found.")
	}
	fmt.Printf("执行账户 UID / Run account UID: %d\n", os.Geteuid())
	c, err := loadConfig(l.ConfigPath)
	if err == nil {
		fmt.Printf("检查间隔 / Check interval: %d 分钟 / minutes（%d 秒 / seconds）\n", c.Interval/60, c.Interval)
	} else if os.IsNotExist(err) {
		fmt.Println("检查间隔 / Check interval: 未找到配置。 / Configuration not found.")
	} else {
		fmt.Println("检查间隔 / Check interval: 无法读取，请重新安装。 / Unavailable; reinstall.")
	}
	fmt.Println("最近的监控日志 / Recent monitoring log entries:")
	body, err := readManaged(l.LogPath, true, 2*1024*1024)
	if err == nil {
		lines := strings.Split(strings.TrimSuffix(string(body), "\n"), "\n")
		if len(lines) > 30 {
			lines = lines[len(lines)-30:]
		}
		fmt.Println(strings.Join(lines, "\n"))
	} else if os.IsNotExist(err) {
		fmt.Println("未找到日志。 / Log file not found.")
	} else {
		fmt.Println("无法读取日志。 / Log file unavailable.")
	}
	return nil
}
func safeProperty(s string) string {
	if regexp.MustCompile(`^[a-z][a-z-]{0,31}$`).MatchString(s) {
		return s
	}
	return "unknown"
}
func uninstall(ctx context.Context, l Layout, m TaskManager, store Store) error {
	p, err := m.State(ctx)
	if err != nil {
		return err
	}
	if err = ownedUnit(l, p, store); err != nil {
		return err
	}
	_, fileErr := store.Read(l.UnitPath, false, 262144)
	if p["LoadState"] == "not-found" && os.IsNotExist(fileErr) {
		fmt.Println("未找到任务。 / Task not found.")
	} else {
		if err = m.Run(ctx, "stop", l.Unit); err != nil {
			return err
		}
		if err = m.Run(ctx, "disable", l.Unit); err != nil {
			return err
		}
		if err = store.Remove(l.UnitPath, false, 262144); err != nil {
			return err
		}
		if err = m.Run(ctx, "daemon-reload"); err != nil {
			return err
		}
		_ = m.Run(ctx, "reset-failed", l.Unit)
		fmt.Println("卸载完成。 / Uninstalled.")
	}
	fmt.Println("保留的文件 / Retained files: " + l.CodeDir + "; " + l.DataDir)
	return nil
}
func monitor(ctx context.Context, l Layout, once bool) error {
	c, err := loadConfig(l.ConfigPath)
	if err != nil {
		return err
	}
	key, err := readManaged(l.KeyPath, true, 64)
	if err != nil {
		return err
	}
	password, err := openConfig(c, key)
	if err != nil {
		return err
	}
	log := FileLog{l.LogPath, once}
	p := newPortal(c, password, log, ctx)
	defer func() { p.Password = ""; password = "" }()
	if err = log.Emit(fmt.Sprintf("MONITOR_STARTED 监控已启动 / Monitor started; uid=%d; interval=%ds", os.Getuid(), c.Interval)); err != nil {
		return err
	}
	if mode == "link2PC" && !once {
		if err = sleepContext(ctx, 60*time.Second); err != nil {
			return nil
		}
	}
	for ctx.Err() == nil {
		ok := false
		active := true
		if mode == "link2user" {
			check, cancel := context.WithTimeout(ctx, 5*time.Second)
			active = hasLoginSession(check)
			cancel()
		}
		if !active {
			if err = log.Emit("SESSION_INACTIVE 用户未登录，已跳过检查。 / User is not signed in; check skipped."); err != nil {
				return err
			}
		} else {
			ok = p.check()
		}
		if once {
			if !ok {
				return fail("检查或登录未成功。 / Check or login did not succeed.")
			}
			return nil
		}
		if sleepContext(ctx, time.Duration(c.Interval)*time.Second) != nil {
			return nil
		}
	}
	return nil
}
func mainRun() (exit int) {
	defer func() {
		if v := recover(); v != nil {
			if e, ok := v.(controlled); ok {
				fmt.Println("错误 / Error: " + e.Message)
			} else {
				fmt.Println("操作失败。 / Operation failed.")
			}
			exit = 1
		}
	}()
	args := os.Args[1:]
	if len(args) == 1 && args[0] == "--version" {
		fmt.Printf("LiteBIT_Net %s %s linux/%s\n", buildVersion, mode, runtime.GOARCH)
		return 0
	}
	if len(args) == 0 || (len(args) == 1 && (args[0] == "--help" || args[0] == "-h")) {
		fmt.Println("LiteBIT_Net " + mode + "\n用法 / Usage: program install|status|uninstall|monitor [--once]\n--once: 仅日常检查，不主动注销 / Routine check only; never force sign-out")
		return 0
	}
	action := args[0]
	once := len(args) == 2 && args[1] == "--once" && action == "monitor"
	if (action != "install" && action != "status" && action != "uninstall" && action != "monitor") || len(args) > 2 || (len(args) == 2 && !once) {
		fmt.Println("参数无效。 / Invalid arguments.")
		return 1
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := preflight(ctx); err != nil {
		fmt.Println("错误 / Error: " + err.Error())
		return 1
	}
	privateUmask()
	l, err := makeLayout()
	if err != nil {
		fmt.Println("错误 / Error: " + err.Error())
		return 1
	}
	m := Manager{l}
	if action == "install" || action == "uninstall" {
		unlock, e := acquireOperationLock()
		if e != nil {
			fmt.Println("错误 / Error: " + e.Error())
			return 1
		}
		defer unlock()
	}
	switch action {
	case "install":
		err = install(ctx, l, m)
	case "status":
		err = showStatus(ctx, l, m)
	case "uninstall":
		err = uninstall(ctx, l, m, diskStore{})
	case "monitor":
		err = monitor(ctx, l, once)
	}
	if err != nil {
		fmt.Println("错误 / Error: " + err.Error())
		return 1
	}
	return 0
}
func main() { os.Exit(mainRun()) }
