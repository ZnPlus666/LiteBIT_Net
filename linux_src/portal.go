package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type response map[string]any

func (r response) text(key string) string { s, _ := r[key].(string); return s }

type PortalError struct{ Code string }

func (e PortalError) Error() string { return e.Code }
func safeCode(s string) string {
	switch s {
	case "ok", "not_online", "not_online_error", "password_error", "user_not_exist", "ip_already_online_error", "logout_error":
		return s
	}
	if regexp.MustCompile(`^E[0-9]{3,6}$`).MatchString(s) {
		return s
	}
	if s == "" {
		return "missing_status"
	}
	return "unknown_status"
}
func safeIP(s string) string {
	if ip := net.ParseIP(s); ip != nil {
		return ip.String()
	}
	return "unknown"
}
func offline(s string) bool { return s == "not_online" || s == "not_online_error" }
func parseResponse(body []byte) (response, error) {
	s := strings.TrimSpace(string(body))
	if !strings.HasPrefix(s, "{") {
		match := regexp.MustCompile(`(?s)^[A-Za-z_$][A-Za-z0-9_.$]*\((.*)\)\s*;?$`).FindStringSubmatch(s)
		if match == nil {
			return nil, PortalError{"request_failed"}
		}
		s = match[1]
	}
	var r response
	if json.Unmarshal([]byte(s), &r) != nil || r == nil {
		return nil, PortalError{"request_failed"}
	}
	return r, nil
}

type Logger interface{ Emit(string) error }
type Portal struct {
	Config      Config
	Password    string
	Log         Logger
	Context     context.Context
	Session     func(context.Context) bool
	RequestHook func(string, url.Values, time.Duration) (response, error)
	RouteHook   func() (string, error)
	Now         func() time.Time
	Sleep       func(time.Duration)
}

func newPortal(c Config, password string, log Logger, ctx context.Context) *Portal {
	p := &Portal{Config: c, Password: password, Log: log, Context: ctx, Now: time.Now}
	p.Sleep = func(d time.Duration) {
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
		}
	}
	if mode == "link2user" {
		p.Session = hasLoginSession
	}
	return p
}
func (p *Portal) emit(s string) {
	if err := p.Log.Emit(s); err != nil {
		panic(controlled{err.Error()})
	}
}
func (p *Portal) routeIP() (string, error) {
	if p.RouteHook != nil {
		return p.RouteHook()
	}
	u, _ := url.Parse(p.Config.Server)
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	conn, err := net.DialTimeout("udp4", net.JoinHostPort(u.Hostname(), port), time.Second)
	if err != nil {
		return "", PortalError{"route_unavailable"}
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}
func (p *Portal) request(path string, params url.Values, budget time.Duration) (response, error) {
	if p.RequestHook != nil {
		return p.RequestHook(path, params, budget)
	}
	ctx, cancel := context.WithTimeout(p.Context, budget)
	defer cancel()
	if ctx.Err() != nil {
		return nil, PortalError{"operation_cancelled"}
	}
	if p.Session != nil && !p.Session(ctx) {
		if ctx.Err() != nil {
			return nil, PortalError{"request_timeout"}
		}
		return nil, PortalError{"session_inactive"}
	}
	if ctx.Err() != nil {
		return nil, PortalError{"request_timeout"}
	}
	u, err := url.Parse(p.Config.Server)
	if err != nil {
		return nil, PortalError{"request_failed"}
	}
	q := url.Values{}
	for k, values := range params {
		q[k] = append([]string(nil), values...)
	}
	stamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	q.Set("callback", "jQuery_litebit_"+stamp)
	q.Set("_", stamp)
	u.Path = strings.TrimRight(u.Path, "/") + path
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, PortalError{"request_failed"}
	}
	req.Header.Set("User-Agent", "LiteBIT_Net/Linux")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Referer", p.Config.Server+"/srun_portal_pc?ac_id="+p.Config.AcID+"&theme=bit")
	// No proxy environment, no redirects, no retries, no logged credential URLs.
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, DialContext: (&net.Dialer{Timeout: budget}).DialContext, ResponseHeaderTimeout: budget, MaxResponseHeaderBytes: 65536}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		if p.Context.Err() != nil {
			return nil, PortalError{"operation_cancelled"}
		}
		if ctx.Err() != nil {
			return nil, PortalError{"request_timeout"}
		}
		return nil, PortalError{"request_failed"}
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, PortalError{"http_" + strconv.Itoa(resp.StatusCode)}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if ctx.Err() != nil {
		return nil, PortalError{"request_timeout"}
	}
	if err != nil || len(body) > 65536 {
		return nil, PortalError{"request_failed"}
	}
	return parseResponse(body)
}
func (p *Portal) query() (response, error) {
	return p.request("/cgi-bin/rad_user_info", nil, 8*time.Second)
}
func errorCode(err error) string {
	if e, ok := err.(PortalError); ok {
		return e.Code
	}
	return "request_failed"
}
func (p *Portal) confirmation() string {
	started := p.Now()
	codes := []string{}
	for attempt := 0; attempt < 5; attempt++ {
		if delay := time.Duration(attempt)*time.Second - p.Now().Sub(started); delay > 0 {
			p.Sleep(delay)
		}
		remaining := 5*time.Second - p.Now().Sub(started)
		if remaining <= 0 {
			break
		}
		r, err := p.request("/cgi-bin/rad_user_info", nil, remaining)
		code := ""
		if p.Context.Err() != nil {
			return "Failed"
		}
		if p.Now().Sub(started) >= 5*time.Second {
			code = "request_timeout"
		} else if err != nil {
			code = errorCode(err)
		} else if r.text("error") == "ok" {
			if r.text("user_name") == p.Config.Username {
				p.emit("LOGIN_OK 自动登录已核实成功 / Automatic login verified; IP=" + safeIP(r.text("online_ip")))
				return "Verified"
			}
			code = "account_mismatch"
			if r.text("user_name") == "" {
				code = "account_missing"
			}
		} else {
			code = safeCode(r.text("error"))
		}
		found := false
		for _, old := range codes {
			if old == code {
				found = true
			}
		}
		if !found {
			codes = append(codes, code)
		}
	}
	if remaining := 5*time.Second - p.Now().Sub(started); remaining > 0 {
		p.Sleep(remaining)
	}
	if p.Context.Err() != nil {
		return "Failed"
	}
	if len(codes) == 0 {
		codes = append(codes, "request_timeout")
	}
	p.emit("VERIFY_UNKNOWN 验证结果未知。 / Verification result unknown.")
	p.emit("VERIFY_STATUS 状态码 / Status code: " + strings.Join(codes, ", "))
	return "Unknown"
}
func (p *Portal) login(strict bool) string {
	ip, err := p.routeIP()
	if err != nil {
		p.emit("LOGIN_FAILED 自动登录失败 / Automatic login failed; code=" + errorCode(err))
		return "Failed"
	}
	r, err := p.request("/cgi-bin/get_challenge", url.Values{"username": {p.Config.Username}, "ip": {ip}}, 8*time.Second)
	if err != nil || r.text("error") != "ok" || r.text("challenge") == "" {
		p.emit("CHALLENGE_FAILED 获取认证挑战值失败，下次检查时重试。 / Failed to obtain the authentication challenge; retry on the next check.")
		return "Failed"
	}
	if r.text("client_ip") != "" {
		ip = safeIP(r.text("client_ip"))
		if ip == "unknown" {
			return "Failed"
		}
	}
	r, err = p.request("/cgi-bin/srun_portal", loginParameters(p.Config.Username, p.Password, ip, r.text("challenge"), p.Config.AcID), 8*time.Second)
	if err != nil {
		p.emit("LOGIN_FAILED 自动登录失败 / Automatic login failed; code=" + errorCode(err))
		return "Failed"
	}
	if r.text("error") != "ok" || strings.Contains(strings.ToLower(r.text("suc_msg")), "already_online") {
		p.emit("LOGIN_FAILED 自动登录失败 / Automatic login failed; code=" + safeCode(r.text("error")) + "；下次检查时重试。 / Retry on the next check.")
		return "Failed"
	}
	p.emit("LOGIN_ACCEPTED 登录请求已接受，正在核实认证状态。 / Login request accepted; verifying authentication status.")
	if strict {
		return p.confirmation()
	}
	r, err = p.query()
	if err == nil && r.text("error") == "ok" && r.text("user_name") == p.Config.Username {
		p.emit("LOGIN_OK 自动登录已核实成功 / Automatic login verified; IP=" + safeIP(r.text("online_ip")))
		return "Verified"
	}
	p.emit("LOGIN_NOT_CONFIRMED 尚未确认认证成功，下次检查时重试。 / Authentication is not confirmed; retry on the next check.")
	return "Failed"
}
func (p *Portal) check() bool {
	r, err := p.query()
	if err != nil {
		p.emit("CHECK_FAILED 检查或登录失败 / Check or login failed; code=" + errorCode(err))
		return false
	}
	if r.text("error") == "ok" {
		p.emit("ONLINE 已认证 / Authenticated; IP=" + safeIP(r.text("online_ip")))
		return true
	}
	if !offline(r.text("error")) {
		p.emit("CHECK_UNKNOWN 无法确认认证状态，下次检查时重试。 / Cannot confirm authentication status; retry on the next check.")
		return false
	}
	p.emit("OFFLINE 未认证，正在登录 / Unauthenticated; logging in.")
	return p.login(false) == "Verified"
}
func (p *Portal) verify(allowDisconnect bool) string {
	r, err := p.query()
	if err != nil {
		p.emit("VERIFY_FAILED 新凭据验证未成功 / New credential verification did not succeed; code=" + errorCode(err))
		return "Failed"
	}
	if r.text("error") == "ok" {
		if !allowDisconnect {
			p.emit("VERIFY_ONLINE 当前已在线，新凭据尚未验证。 / Already authenticated; new credentials have not been verified.")
			return "DisconnectRequired"
		}
		ip := safeIP(r.text("online_ip"))
		if ip == "unknown" {
			ip, err = p.routeIP()
			if err != nil {
				return "Failed"
			}
		}
		params := url.Values{"action": {"logout"}, "ac_id": {p.Config.AcID}, "ip": {ip}}
		if name := r.text("user_name"); name != "" {
			params.Set("username", name)
		}
		p.emit("VERIFY_LOGOUT 正在注销当前校园网…… / Signing out of the current campus-network session...")
		result, e := p.request("/cgi-bin/srun_portal", params, 8*time.Second)
		if e != nil || (result.text("error") != "ok" && result.text("error") != "logout_ok") {
			p.emit("VERIFY_LOGOUT_FAILED 注销失败，未验证新凭据。 / Sign-out failed; new credentials were not verified.")
			return "Failed"
		}
		for attempt := 0; attempt < 5; attempt++ {
			r, err = p.query()
			if err != nil {
				return "Failed"
			}
			if offline(r.text("error")) {
				break
			}
			if r.text("error") != "ok" {
				return "Failed"
			}
			if attempt < 4 {
				p.Sleep(500 * time.Millisecond)
			}
		}
	}
	if !offline(r.text("error")) {
		p.emit("VERIFY_NOT_OFFLINE 未确认下线，未验证新凭据。 / Sign-out was not confirmed; new credentials were not verified.")
		return "Failed"
	}
	p.emit("VERIFY_LOGIN 正在使用新凭据登录…… / Logging in with the new credentials...")
	result := p.login(true)
	if result == "Verified" {
		p.emit("CREDENTIALS_VALID 新凭据验证成功。 / New credentials verified.")
	}
	return result
}
