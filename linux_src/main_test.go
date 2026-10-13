package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"debug/elf"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type memoryLog struct{ Lines []string }

func (l *memoryLog) Emit(s string) error { l.Lines = append(l.Lines, s); return nil }
func testConfig() Config {
	return Config{Format: 2, Mode: mode, Server: "http://10.0.0.55", Username: "synthetic-user", AcID: "8", Interval: 600}
}
func eachMode(t *testing.T, fn func(*testing.T)) {
	old := mode
	defer func() { mode = old }()
	for _, name := range []string{"link2user", "link2PC"} {
		mode = name
		t.Run(name, fn)
	}
}
func TestProtocolVectors(t *testing.T) {
	body, err := os.ReadFile("../tests_linux/crypto_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Username, Password, IP, Token, Acid string
		Expected                            map[string]string
	}
	if err = json.Unmarshal(body, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors) != 19 {
		t.Fatal("missing vectors")
	}
	eachMode(t, func(t *testing.T) {
		for i, v := range vectors {
			actual := loginParameters(v.Username, v.Password, v.IP, v.Token, v.Acid)
			for k, want := range v.Expected {
				if k == "os" || k == "name" {
					want = "Linux"
				}
				if actual.Get(k) != want {
					t.Fatalf("vector %d field %s differs", i, k)
				}
			}
		}
	})
}
func TestCredentialRoundTripAndRandomness(t *testing.T) {
	eachMode(t, func(t *testing.T) {
		key := []byte(strings.Repeat("k", 32))
		c := testConfig()
		password := `synthetic "password" <>&`
		sealed, err := sealConfig(c, password, key)
		if err != nil {
			t.Fatal(err)
		}
		other, err := sealConfig(c, password, key)
		if err != nil {
			t.Fatal(err)
		}
		if sealed.Nonce == other.Nonce || sealed.PasswordEncrypted == other.PasswordEncrypted {
			t.Fatal("encryption not randomized")
		}
		got, err := openConfig(sealed, key)
		if err != nil || got != password {
			t.Fatal("round trip failed")
		}
		encoded, _ := json.Marshal(sealed)
		if strings.Contains(string(encoded), password) {
			t.Fatal("plaintext password persisted")
		}
	})
}
func TestCredentialTampering(t *testing.T) {
	eachMode(t, func(t *testing.T) {
		key := []byte(strings.Repeat("k", 32))
		sealed, err := sealConfig(testConfig(), "synthetic-password", key)
		if err != nil {
			t.Fatal(err)
		}
		changes := []func(*Config){func(c *Config) { c.Username = "different-user" }, func(c *Config) { c.Server = "http://10.0.0.56" }, func(c *Config) { c.AcID = "9" }, func(c *Config) { c.Interval = 1200 }, func(c *Config) { c.Mode = "wrong" }, func(c *Config) { c.Format = 1 }, func(c *Config) { c.Nonce = "bad" }, func(c *Config) { c.PasswordEncrypted = base64.StdEncoding.EncodeToString(make([]byte, 32)) }}
		for i, change := range changes {
			c := sealed
			change(&c)
			if _, err = openConfig(c, key); err == nil {
				t.Fatalf("tamper %d accepted", i)
			}
		}
		for _, badKey := range [][]byte{nil, []byte("short"), []byte(strings.Repeat("x", 32))} {
			if _, err = openConfig(sealed, badKey); err == nil {
				t.Fatal("wrong key accepted")
			}
		}
	})
}
func TestConfigAndIntervalValidation(t *testing.T) {
	for _, value := range []string{"", "  ", "10"} {
		got, err := parseInterval(value)
		if err != nil || got != 600 {
			t.Fatal("default interval")
		}
	}
	for _, value := range []string{"0", "-1", "1.5", "35791395", "100000000", "abc"} {
		if _, err := parseInterval(value); err == nil {
			t.Fatal("invalid interval accepted")
		}
	}
	eachMode(t, func(t *testing.T) {
		for _, server := range []string{"http://example.com", "http://user:pass@10.0.0.55", "http://10.0.0.55/?x=y", "http://10.0.0.55/#f", "ftp://10.0.0.55", "http://[::1]", "http://10.0.0.55:99999", "http://10.0.0.55:0"} {
			c := testConfig()
			c.Server = server
			if validateConfig(c) == nil {
				t.Fatalf("invalid server %s accepted", server)
			}
		}
		for _, pw := range []string{"", "bad\ninput", "非ASCII", strings.Repeat("p", 257)} {
			if validateCredentials("synthetic", pw) == nil {
				t.Fatal("invalid password accepted")
			}
		}
		c := testConfig()
		c.Interval = 61
		if validateConfig(c) == nil {
			t.Fatal("invalid interval config accepted")
		}
	})
}

type call struct {
	Path   string
	Params url.Values
	Budget time.Duration
}

func scriptedPortal(replies []response) (*Portal, *memoryLog, *[]call, *time.Time) {
	clock := time.Unix(100, 0)
	log := &memoryLog{}
	calls := []call{}
	p := newPortal(testConfig(), "synthetic-password", log, context.Background())
	p.Session = nil
	p.RouteHook = func() (string, error) { return "10.1.2.3", nil }
	p.Now = func() time.Time { return clock }
	p.Sleep = func(d time.Duration) { clock = clock.Add(d) }
	p.RequestHook = func(path string, params url.Values, budget time.Duration) (response, error) {
		calls = append(calls, call{path, params, budget})
		if len(replies) == 0 {
			return nil, PortalError{"request_failed"}
		}
		r := replies[0]
		replies = replies[1:]
		return r, nil
	}
	return p, log, &calls, &clock
}
func challenge() response {
	return response{"error": "ok", "challenge": "0123456789abcdef0123456789abcdef01234567", "client_ip": "10.1.2.3"}
}
func online(name string) response {
	return response{"error": "ok", "user_name": name, "online_ip": "10.1.2.3"}
}
func TestOnlineIsNotCredentialVerification(t *testing.T) {
	eachMode(t, func(t *testing.T) {
		p, _, calls, _ := scriptedPortal([]response{online("other-account")})
		if p.verify(false) != "DisconnectRequired" || len(*calls) != 1 {
			t.Fatal("online falsely verified")
		}
		p, _, calls, _ = scriptedPortal([]response{online("other-account")})
		if !p.check() || len(*calls) != 1 {
			t.Fatal("routine online issued login")
		}
	})
}
func TestLogoutUsesCurrentAccountBeforeFreshChallenge(t *testing.T) {
	eachMode(t, func(t *testing.T) {
		p, _, calls, _ := scriptedPortal([]response{online("current-account"), {"error": "ok"}, {"error": "not_online"}, challenge(), {"error": "ok"}, online("synthetic-user")})
		if p.verify(true) != "Verified" {
			t.Fatal("valid verification failed")
		}
		if len(*calls) != 6 || (*calls)[1].Params.Get("action") != "logout" || (*calls)[1].Params.Get("username") != "current-account" || (*calls)[3].Path != "/cgi-bin/get_challenge" || (*calls)[4].Params.Get("username") != "synthetic-user" {
			t.Fatal("wrong authentication sequence")
		}
	})
}
func TestExplicitRejectionDoesNotOfferUnknown(t *testing.T) {
	eachMode(t, func(t *testing.T) {
		for _, result := range []response{{"error": "password_error"}, {"error": "ok", "suc_msg": "already_online"}, {"error": "user_not_exist"}} {
			p, log, calls, _ := scriptedPortal([]response{{"error": "not_online"}, challenge(), result})
			if p.verify(false) != "Failed" || len(*calls) != 3 || strings.Contains(strings.Join(log.Lines, "\n"), "VERIFY_UNKNOWN") {
				t.Fatal("rejected password treated unknown")
			}
		}
		p, _, calls, _ := scriptedPortal([]response{online("current"), {"error": "logout_error"}})
		if p.verify(true) != "Failed" || len(*calls) != 2 {
			t.Fatal("failed logout ignored")
		}
		p, _, _, _ = scriptedPortal([]response{online("current"), {"error": "ok"}, online("current"), online("current"), online("current"), online("current"), online("current")})
		if p.verify(true) != "Failed" {
			t.Fatal("still online verified")
		}
	})
}
func TestConfirmationPollsAndStopsOnMatch(t *testing.T) {
	eachMode(t, func(t *testing.T) {
		p, _, calls, clock := scriptedPortal([]response{{"error": "not_online"}, online("other"), online("synthetic-user")})
		if p.confirmation() != "Verified" || len(*calls) != 3 || clock.Sub(time.Unix(100, 0)) != 2*time.Second {
			t.Fatal("polling did not stop on match")
		}
		p, log, calls, clock := scriptedPortal([]response{{"error": "not_online"}, online("other"), {"error": "ok"}, {"error": "untrusted-password=secret"}, {"error": "E1234"}})
		if p.confirmation() != "Unknown" || len(*calls) != 5 || clock.Sub(time.Unix(100, 0)) != 5*time.Second {
			t.Fatal("five second polling bound")
		}
		output := strings.Join(log.Lines, "\n")
		if strings.Contains(output, "secret") || !strings.Contains(output, "account_mismatch") || !strings.Contains(output, "account_missing") || !strings.Contains(output, "E1234") {
			t.Fatal("unsafe or missing diagnostics")
		}
	})
}
func TestConfirmationSlowRequestConsumesDeadline(t *testing.T) {
	p, _, calls, clock := scriptedPortal(nil)
	p.RequestHook = func(path string, params url.Values, budget time.Duration) (response, error) {
		*calls = append(*calls, call{path, params, budget})
		*clock = clock.Add(budget)
		return online("synthetic-user"), nil
	}
	if p.confirmation() != "Unknown" || len(*calls) != 1 || clock.Sub(time.Unix(100, 0)) != 5*time.Second {
		t.Fatal("late result accepted")
	}
}
func TestSkipAndUnknownChoices(t *testing.T) {
	eachMode(t, func(t *testing.T) {
		for _, answer := range []string{"", "N", "anything"} {
			p, _, calls, _ := scriptedPortal([]response{online("current")})
			if err := setupVerification(p, func(string) (string, error) { return answer, nil }); err != nil || len(*calls) != 1 {
				t.Fatal("skip did not skip")
			}
		}
		for _, answer := range []string{"Y", "y", "N", "", "anything"} {
			p, _, _, _ := scriptedPortal([]response{{"error": "not_online"}, challenge(), {"error": "ok"}, online("other"), online("other"), online("other"), online("other"), online("other")})
			err := setupVerification(p, func(string) (string, error) { return answer, nil })
			if (err == nil) != strings.EqualFold(answer, "Y") {
				t.Fatal("unknown choice mishandled")
			}
		}
	})
}
func TestHTTPDirectJSONJSONPAndRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "http://127.0.0.1:1/never", 302)
			return
		}
		if r.URL.Path == "/jsonp" {
			fmt.Fprint(w, `jQuery_123({"error":"ok"});`)
			return
		}
		fmt.Fprint(w, `{"error":"not_online"}`)
	}))
	defer server.Close()
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	p := newPortal(testConfig(), "synthetic", &memoryLog{}, context.Background())
	p.Config.Server = server.URL
	p.Session = nil
	for _, path := range []string{"/normal", "/jsonp"} {
		if _, err := p.request(path, nil, time.Second); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.request("/redirect", nil, time.Second); errorCode(err) != "http_302" {
		t.Fatal("redirect followed")
	}
	for _, body := range []string{"[]", "null", "bad", "jQuery([]);", "{} {}"} {
		if _, err := parseResponse([]byte(body)); err == nil {
			t.Fatal("invalid response accepted")
		}
	}
}
func TestHTTPTimeoutsBoundHeadersAndBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/headers" {
			time.Sleep(200 * time.Millisecond)
		} else {
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			time.Sleep(200 * time.Millisecond)
		}
		fmt.Fprint(w, `{"error":"ok"}`)
	}))
	defer server.Close()
	p := newPortal(testConfig(), "synthetic", &memoryLog{}, context.Background())
	p.Config.Server = server.URL
	p.Session = nil
	for _, path := range []string{"/headers", "/body"} {
		start := time.Now()
		if _, err := p.request(path, nil, 50*time.Millisecond); errorCode(err) != "request_timeout" {
			t.Fatal("missing timeout")
		}
		if time.Since(start) > 500*time.Millisecond {
			t.Fatal("timeout exceeded")
		}
	}
}
func TestHTTPResponseSizeBound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, strings.Repeat("x", 65537)) }))
	defer server.Close()
	p := newPortal(testConfig(), "synthetic", &memoryLog{}, context.Background())
	p.Config.Server = server.URL
	p.Session = nil
	if _, err := p.request("/large", nil, time.Second); err == nil {
		t.Fatal("oversize response accepted")
	}
}
func TestSessionGateAndCancellationBeforeNetwork(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; fmt.Fprint(w, `{}`) }))
	defer server.Close()
	p := newPortal(testConfig(), "synthetic", &memoryLog{}, context.Background())
	p.Config.Server = server.URL
	p.Session = func(context.Context) bool { return false }
	if _, err := p.request("/deny", nil, time.Second); errorCode(err) != "session_inactive" {
		t.Fatal("no-session request not rejected")
	}
	p.Session = func(ctx context.Context) bool { <-ctx.Done(); return true }
	if _, err := p.request("/slow-session", nil, 10*time.Millisecond); errorCode(err) != "request_timeout" {
		t.Fatal("session budget not enforced")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p.Context = ctx
	p.Session = nil
	if _, err := p.request("/cancel", nil, time.Second); errorCode(err) != "operation_cancelled" {
		t.Fatal("cancelled request sent")
	}
	if requests != 0 {
		t.Fatal("gate allowed network")
	}
}

func TestSessionIdentityClassesAndClosingState(t *testing.T) {
	for _, example := range []struct {
		uid, class, state string
		want              bool
	}{
		{"1234", "user", "active", true}, {"1234", "user", "online", true}, {"1234", "user-early", "active", true},
		{"1234", "user-light", "active", true}, {"1234", "user-early-light", "active", true},
		{"5678", "user", "active", false}, {"1234", "user", "closing", false}, {"1234", "manager", "active", false},
		{"1234", "background", "active", false}, {"1234", "greeter", "active", false}, {"1234", "user", "", false},
	} {
		p := map[string]string{"User": example.uid, "Class": example.class, "State": example.state}
		if sessionMatches("1234", p) != example.want {
			t.Fatal("incorrect login session accepted")
		}
	}
	if sessionMatches("1234", map[string]string{}) {
		t.Fatal("missing session accepted")
	}
}

type memoryStore struct {
	Files    map[string][]byte
	Perms    map[string]os.FileMode
	FailPath string
	Events   []string
}

func (s *memoryStore) Prepare(Layout) error { return nil }
func (s *memoryStore) Read(path string, _ bool, _ int64) ([]byte, error) {
	b, ok := s.Files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return append([]byte(nil), b...), nil
}
func (s *memoryStore) Write(path string, b []byte, perm os.FileMode) error {
	s.Events = append(s.Events, "write:"+path)
	if s.FailPath == path {
		s.FailPath = ""
		return errors.New("synthetic write failure")
	}
	s.Files[path] = append([]byte(nil), b...)
	s.Perms[path] = perm
	return nil
}
func (s *memoryStore) Remove(path string, _ bool, _ int64) error {
	s.Events = append(s.Events, "remove:"+path)
	delete(s.Files, path)
	return nil
}

type fakeManager struct {
	Props  map[string]string
	Events []string
	Fail   string
}

func (m *fakeManager) State(context.Context) (map[string]string, error) {
	p := map[string]string{}
	for k, v := range m.Props {
		p[k] = v
	}
	return p, nil
}
func (m *fakeManager) Run(_ context.Context, args ...string) error {
	m.Events = append(m.Events, args[0])
	if args[0] == m.Fail {
		m.Fail = ""
		return errors.New("synthetic failure")
	}
	switch args[0] {
	case "stop":
		m.Props["ActiveState"] = "inactive"
	case "start":
		m.Props["ActiveState"] = "active"
	case "enable":
		m.Props["UnitFileState"] = "enabled"
	case "disable":
		m.Props["UnitFileState"] = "disabled"
	}
	return nil
}
func (m *fakeManager) WaitActive(context.Context) error {
	if m.Props["ActiveState"] != "active" {
		return errors.New("synthetic inactive")
	}
	return nil
}
func fixture(existing bool) (Layout, *memoryStore, *fakeManager, map[string][]byte) {
	l := Layout{Unit: "litebit-net-" + mode + ".service", CodeDir: "/synthetic/code", DataDir: "/synthetic/data", UnitPath: "/synthetic/units/service", CodePath: "/synthetic/code/program", ConfigPath: "/synthetic/data/config.json", KeyPath: "/synthetic/data/credential.key", LogPath: "/synthetic/data/log"}
	s := &memoryStore{Files: map[string][]byte{}, Perms: map[string]os.FileMode{}}
	m := &fakeManager{Props: map[string]string{"LoadState": "not-found", "ActiveState": "inactive", "UnitFileState": ""}}
	if existing {
		m.Props = map[string]string{"LoadState": "loaded", "ActiveState": "active", "UnitFileState": "enabled", "FragmentPath": l.UnitPath}
		s.Files[l.UnitPath] = []byte(marker() + "\nold unit")
		s.Files[l.ConfigPath] = []byte("old encrypted config")
		s.Files[l.KeyPath] = []byte(strings.Repeat("k", 32))
		s.Files[l.CodePath] = []byte("old binary")
	}
	before := map[string][]byte{}
	for k, b := range s.Files {
		before[k] = append([]byte(nil), b...)
	}
	return l, s, m, before
}
func TestInstallVerificationOrderAndRollback(t *testing.T) {
	eachMode(t, func(t *testing.T) {
		for _, existing := range []bool{false, true} {
			for _, stage := range []string{"pass", "skip", "unknown-continue", "reject", "cancel", "write", "enable", "start", "panic"} {
				t.Run(fmt.Sprintf("%t_%s", existing, stage), func(t *testing.T) {
					l, s, m, before := fixture(existing)
					if stage == "write" {
						s.FailPath = l.ConfigPath
					}
					if stage == "start" || stage == "enable" {
						m.Fail = stage
					}
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					verify := func() error {
						if m.Props["ActiveState"] != "inactive" || !reflect.DeepEqual(s.Files, before) {
							t.Fatal("verification did not precede writes or pause old task")
						}
						if stage == "reject" {
							return fail("synthetic rejected")
						}
						if stage == "cancel" {
							cancel()
							return fail("synthetic cancelled")
						}
						if stage == "panic" {
							panic(controlled{"synthetic failure"})
						}
						return nil
					}
					var err error
					panicked := false
					func() {
						defer func() {
							if recover() != nil {
								panicked = true
							}
						}()
						err = installConfig(ctx, l, m, s, testConfig(), "synthetic-password", []byte("new binary"), verify)
					}()
					failed := stage == "reject" || stage == "cancel" || stage == "write" || stage == "enable" || stage == "start" || stage == "panic"
					if failed {
						if err == nil && !panicked {
							t.Fatal("failure ignored")
						}
						if !reflect.DeepEqual(s.Files, before) {
							t.Fatal("files not restored")
						}
						if existing && m.Props["ActiveState"] != "active" {
							t.Fatal("old task not restarted")
						}
					} else {
						if err != nil || panicked {
							t.Fatal("installation failed")
						}
						var c Config
						if json.Unmarshal(s.Files[l.ConfigPath], &c) != nil || c.Format != 2 || c.Interval != 600 {
							t.Fatal("config not saved")
						}
						if m.Props["ActiveState"] != "active" || s.Perms[l.CodePath] != 0700 || s.Perms[l.KeyPath] != 0600 || s.Perms[l.UnitPath] != 0644 {
							t.Fatal("service or permissions wrong")
						}
					}
				})
			}
		}
	})
}
func TestLegacyConfigurationCanBeReinstalled(t *testing.T) {
	eachMode(t, func(t *testing.T) {
		l, s, m, _ := fixture(true)
		s.Files[l.ConfigPath] = []byte(`{"format":1,"hmac":"synthetic-old","password_encrypted":"synthetic-old"}`)
		delete(s.Files, l.CodePath)
		if err := installConfig(context.Background(), l, m, s, testConfig(), "synthetic-new", []byte("new binary"), func() error { return nil }); err != nil {
			t.Fatal(err)
		}
		var c Config
		if json.Unmarshal(s.Files[l.ConfigPath], &c) != nil || c.Format != 2 {
			t.Fatal("legacy upgrade failed")
		}
	})
}
func TestForeignUnitAndMaskedUnitAreNotModified(t *testing.T) {
	eachMode(t, func(t *testing.T) {
		for _, condition := range []string{"path", "marker", "masked"} {
			l, s, m, before := fixture(true)
			switch condition {
			case "path":
				m.Props["FragmentPath"] = "/foreign/service"
			case "marker":
				s.Files[l.UnitPath] = []byte("foreign")
				before[l.UnitPath] = []byte("foreign")
			case "masked":
				m.Props["LoadState"] = "masked"
			}
			verified := false
			err := installConfig(context.Background(), l, m, s, testConfig(), "synthetic", []byte("new"), func() error { verified = true; return nil })
			if err == nil || verified || len(m.Events) != 0 || !reflect.DeepEqual(s.Files, before) {
				t.Fatal("foreign task modified")
			}
		}
	})
}
func TestUninstallRetainsCredentials(t *testing.T) {
	eachMode(t, func(t *testing.T) {
		l, s, m, _ := fixture(true)
		if err := uninstall(context.Background(), l, m, s); err != nil {
			t.Fatal(err)
		}
		if _, ok := s.Files[l.UnitPath]; ok {
			t.Fatal("unit retained")
		}
		for _, path := range []string{l.ConfigPath, l.KeyPath, l.CodePath} {
			if _, ok := s.Files[path]; !ok {
				t.Fatal("runtime file removed")
			}
		}
	})
}
func TestServiceDefinitionAndQuoting(t *testing.T) {
	eachMode(t, func(t *testing.T) {
		l, _, _, _ := fixture(false)
		text, err := serviceText(l)
		if err != nil {
			t.Fatal(err)
		}
		s := string(text)
		for _, want := range []string{marker(), "UMask=0077", "NoNewPrivileges=true", " monitor", "Restart=on-failure"} {
			if !strings.Contains(s, want) {
				t.Fatal("missing unit property")
			}
		}
		for _, bad := range []string{"python", "openssl", "synthetic-password", "enable-linger"} {
			if strings.Contains(s, bad) {
				t.Fatal("unexpected dependency or secret")
			}
		}
		if mode == "link2PC" {
			if !strings.Contains(s, "WantedBy=multi-user.target") || !strings.Contains(s, "ProtectHome=true") {
				t.Fatal("wrong system unit")
			}
		} else if !strings.Contains(s, "WantedBy=default.target") {
			t.Fatal("wrong user unit")
		}
	})
	quoted, err := unitQuote(`/home/a%$"name/program`)
	if err != nil || !strings.Contains(quoted, "%%") || !strings.Contains(quoted, "$$") {
		t.Fatal("unit expansion not escaped")
	}
	if _, err = unitQuote("bad\npath"); err == nil {
		t.Fatal("control character accepted")
	}
}
func TestDiagnosticsDoNotExposeCredentials(t *testing.T) {
	for _, code := range []string{"password=synthetic-secret", "raw\nresponse", "3120"} {
		if safeCode(code) != "unknown_status" {
			t.Fatal("unsafe code accepted")
		}
	}
	if safeIP("synthetic-password") != "unknown" || safeProperty("secret\ntext") != "unknown" {
		t.Fatal("unsafe field accepted")
	}
	if safeCode("E1234") != "E1234" {
		t.Fatal("safe code dropped")
	}
}

func TestBundledBinariesAreStaticAndMatchChecksums(t *testing.T) {
	for _, name := range []string{"link2user", "link2PC"} {
		folder := filepath.Join("..", "LiteBIT_Net_linux_"+name)
		list, err := os.ReadFile(filepath.Join(folder, "SHA256SUMS"))
		if os.IsNotExist(err) {
			t.Skip("build release binaries first")
		}
		if err != nil {
			t.Fatal(err)
		}
		checksums := map[string]string{}
		for _, line := range strings.Split(strings.TrimSpace(string(list)), "\n") {
			fields := strings.Fields(line)
			if len(fields) != 2 {
				t.Fatal("invalid checksum list")
			}
			checksums[strings.TrimPrefix(fields[1], "*")] = fields[0]
		}
		for _, arch := range []string{"amd64", "arm64"} {
			relative := "bin/litebit_linux_" + name + "_" + arch
			path := filepath.Join(folder, filepath.FromSlash(relative))
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(body)
			if checksums[relative] != hex.EncodeToString(digest[:]) {
				t.Fatal("binary checksum differs")
			}
			file, err := elf.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			want := elf.EM_X86_64
			if arch == "arm64" {
				want = elf.EM_AARCH64
			}
			if file.Machine != want || file.Class != elf.ELFCLASS64 {
				t.Fatal("wrong architecture")
			}
			for _, program := range file.Progs {
				if program.Type == elf.PT_INTERP {
					t.Fatal("dynamic loader dependency")
				}
			}
			libs, err := file.ImportedLibraries()
			if err != nil || len(libs) != 0 {
				t.Fatal("shared library dependency")
			}
			_ = file.Close()
			info, err := buildinfo.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			cgoDisabled := false
			for _, setting := range info.Settings {
				if setting.Key == "CGO_ENABLED" && setting.Value == "0" {
					cgoDisabled = true
				}
			}
			// Trimpath builds intentionally omit linker flags from build metadata.
			// The retained mode-specific version marker identifies each release.
			if !bytes.Contains(body, []byte("native-v2-"+name)) || !cgoDisabled {
				t.Fatal("wrong mode or dynamic build")
			}
		}
	}
}
