// Copyright (c) 2026 LiteBIT_Net contributors. MIT license.
package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5" // Required by the existing SRun protocol, not for stored credentials.
	"crypto/rand"
	"crypto/sha1" // Required by the existing SRun protocol, not for stored credentials.
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"strconv"
)

const maxIntervalMinutes = 35791394
const alphabet = "LVoJPiCN2R8G90yg+hmFHuacZ1OWMnrsSTXkYpUq/3dlbfKwv6xztjI7DeBE45QA"

type Config struct {
	Format            int    `json:"format"`
	Mode              string `json:"mode"`
	Server            string `json:"server"`
	Username          string `json:"username"`
	AcID              string `json:"ac_id"`
	Interval          int64  `json:"check_interval_seconds"`
	PasswordEncrypted string `json:"password_encrypted"`
	Nonce             string `json:"nonce"`
}

func fail(s string) error { return fmt.Errorf("%s", s) }
func words(data []byte, length bool) []uint32 {
	n := (len(data) + 3) / 4
	v := make([]uint32, n)
	for i, b := range data {
		v[i/4] |= uint32(b) << (uint(i%4) * 8)
	}
	if length {
		v = append(v, uint32(len(data)))
	}
	return v
}
func xencode(text, token string) []byte {
	if text == "" {
		return nil
	}
	v, k := words([]byte(text), true), words([]byte(token), false)
	for len(k) < 4 {
		k = append(k, 0)
	}
	z, total := v[len(v)-1], uint32(0)
	for round := 0; round < 6+52/len(v); round++ {
		total += 0x9e3779b9
		e := (total >> 2) & 3
		for p := range v {
			y := v[(p+1)%len(v)]
			m := (z >> 5) ^ (y << 2)
			m += ((y >> 3) ^ (z << 4)) ^ (total ^ y)
			m += k[(uint32(p)&3)^e] ^ z
			v[p] += m
			z = v[p]
		}
	}
	result := make([]byte, len(v)*4)
	for i, w := range v {
		binary.LittleEndian.PutUint32(result[i*4:], w)
	}
	return result
}

// Fixed field order and JSON escaping must match the browser protocol.
func loginParameters(username, password, ip, token, acid string) url.Values {
	infoStruct := struct {
		Username string `json:"username"`
		Password string `json:"password"`
		IP       string `json:"ip"`
		Acid     string `json:"acid"`
		EncVer   string `json:"enc_ver"`
	}{username, password, ip, acid, "srun_bx1"}
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(infoStruct)
	info := "{SRBX1}" + base64.NewEncoding(alphabet).EncodeToString(xencode(string(bytes.TrimSuffix(body.Bytes(), []byte("\n"))), token))
	mac := hmac.New(md5.New, []byte(token))
	_, _ = mac.Write([]byte(password))
	digest := hex.EncodeToString(mac.Sum(nil))
	sum := sha1.Sum([]byte(token + username + token + digest + token + acid + token + ip + token + "200" + token + "1" + token + info))
	return url.Values{"action": {"login"}, "username": {username}, "password": {"{MD5}" + digest}, "ac_id": {acid}, "ip": {ip}, "chksum": {hex.EncodeToString(sum[:])}, "info": {info}, "n": {"200"}, "type": {"1"}, "os": {"Linux"}, "name": {"Linux"}, "double_stack": {"0"}, "ignore": {"2"}}
}

func validateCredentials(username, password string) error {
	if len(username) == 0 || len(username) > 128 {
		return fail("账号为空或过长。 / The account is empty or too long.")
	}
	if len(password) == 0 || len(password) > 256 {
		return fail("密码为空或过长。 / The password is empty or too long.")
	}
	for _, b := range []byte(username + password) {
		if b < 32 || b > 126 {
			return fail("账号和密码仅支持可打印 ASCII 字符。 / Only printable ASCII characters are supported for the account and password.")
		}
	}
	return nil
}
func validateConfig(c Config) error {
	if c.Format != 2 || c.Mode != mode {
		return fail("配置版本或模式无效，请重新安装。 / Invalid configuration version or mode; reinstall.")
	}
	if c.Interval < 60 || c.Interval > maxIntervalMinutes*60 || c.Interval%60 != 0 {
		return fail("检查间隔无效。 / Invalid check interval.")
	}
	u, err := url.Parse(c.Server)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return fail("服务器地址无效。 / Invalid server address.")
	}
	if ip := net.ParseIP(u.Hostname()); ip == nil || ip.To4() == nil {
		return fail("服务器需使用有效的 IPv4 地址。 / The server must use a valid IPv4 address.")
	}
	if u.Port() != "" {
		p, e := strconv.Atoi(u.Port())
		if e != nil || p < 1 || p > 65535 {
			return fail("服务器端口无效。 / Invalid server port.")
		}
	}
	if !regexp.MustCompile(`^[0-9]{1,8}$`).MatchString(c.AcID) {
		return fail("认证参数无效。 / Invalid authentication parameters.")
	}
	return validateCredentials(c.Username, "placeholder")
}

// AES-GCM authenticates all configuration metadata as well as the password.
// The random key is access-controlled on disk, not hardware-bound.
func configAAD(c Config) []byte {
	c.PasswordEncrypted = ""
	c.Nonce = ""
	body, _ := json.Marshal(c)
	return body
}
func credentialCipher(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, fail("密钥文件无效。 / Invalid key file.")
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("LiteBIT_Net AES-GCM credentials v2"))
	block, err := aes.NewCipher(mac.Sum(nil))
	if err != nil {
		return nil, fail("密码加密失败。 / Password encryption failed.")
	}
	return cipher.NewGCM(block)
}
func sealConfig(c Config, password string, key []byte) (Config, error) {
	if err := validateConfig(c); err != nil {
		return c, err
	}
	if err := validateCredentials(c.Username, password); err != nil {
		return c, err
	}
	aead, err := credentialCipher(key)
	if err != nil {
		return c, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return c, fail("无法生成随机数。 / Cannot generate secure random data.")
	}
	c.PasswordEncrypted = base64.StdEncoding.EncodeToString(aead.Seal(nil, nonce, []byte(password), configAAD(c)))
	c.Nonce = base64.StdEncoding.EncodeToString(nonce)
	return c, nil
}
func openConfig(c Config, key []byte) (string, error) {
	if err := validateConfig(c); err != nil {
		return "", err
	}
	aead, err := credentialCipher(key)
	if err != nil {
		return "", err
	}
	nonce, e1 := base64.StdEncoding.Strict().DecodeString(c.Nonce)
	ciphertext, e2 := base64.StdEncoding.Strict().DecodeString(c.PasswordEncrypted)
	if e1 != nil || e2 != nil || len(nonce) != aead.NonceSize() {
		return "", fail("加密配置无效。 / Invalid encrypted configuration.")
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, configAAD(c))
	if err != nil {
		return "", fail("配置完整性验证失败。 / Configuration integrity verification failed.")
	}
	password := string(plaintext)
	for i := range plaintext {
		plaintext[i] = 0
	}
	if err = validateCredentials(c.Username, password); err != nil {
		return "", err
	}
	return password, nil
}
