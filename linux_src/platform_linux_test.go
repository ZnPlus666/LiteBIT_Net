//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxStoragePermissionsAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	if err := secureDirectory(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.json")
	if err := atomicWrite(path, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := readManaged(path, true, 100)
	if err != nil || string(got) != "synthetic" {
		t.Fatal("private file read failed")
	}
	if os.Chmod(path, 0644) != nil {
		t.Fatal("chmod failed")
	}
	if _, err = readManaged(path, true, 100); err == nil {
		t.Fatal("public credential file accepted")
	}
	_ = os.Chmod(path, 0600)
	link := filepath.Join(dir, "link")
	if os.Symlink(path, link) != nil {
		t.Fatal("symlink failed")
	}
	if _, err = readManaged(link, true, 100); err == nil {
		t.Fatal("symlink accepted")
	}
	if _, err = readManaged(path, true, 3); err == nil {
		t.Fatal("file limit ignored")
	}
	if err = atomicWrite(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = readManaged(path, true, 100); err != nil {
		t.Fatal(err)
	}
}
func TestLinuxStatusDoesNotDecryptOrWrite(t *testing.T) {
	eachMode(t, func(t *testing.T) {
		dir := t.TempDir()
		_ = secureDirectory(dir)
		l, _, m, _ := fixture(true)
		l.ConfigPath = filepath.Join(dir, "config.json")
		l.LogPath = filepath.Join(dir, "log")
		c := testConfig()
		c.PasswordEncrypted = "intentionally-not-decryptable"
		body, _ := json.Marshal(c)
		if err := atomicWrite(l.ConfigPath, body, 0600); err != nil {
			t.Fatal(err)
		}
		if err := atomicWrite(l.LogPath, []byte("synthetic log\n"), 0600); err != nil {
			t.Fatal(err)
		}
		old := os.Stdout
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		os.Stdout = w
		err = showStatus(context.Background(), l, m)
		_ = w.Close()
		os.Stdout = old
		out, _ := io.ReadAll(r)
		_ = r.Close()
		if err != nil || !strings.Contains(string(out), "10 分钟 / minutes（600 秒 / seconds）") || !strings.Contains(string(out), "synthetic log") || len(m.Events) != 0 {
			t.Fatal("status mutated state or failed")
		}
		after, _ := os.ReadFile(l.ConfigPath)
		if !bytes.Equal(after, body) {
			t.Fatal("status modified config")
		}
	})
}
