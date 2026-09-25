package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qnqatop/papeer/internal/updater"
)

func TestKeygenSignVerifyRoundtrip(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"keygen"}, &out); err != nil {
		t.Fatal(err)
	}
	keys := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		k, v, _ := strings.Cut(line, "=")
		keys[k] = v
	}
	priv, pub := keys["UPDATE_SIGNING_KEY"], keys["UPDATE_PUBLIC_KEY"]
	if priv == "" || pub == "" {
		t.Fatalf("keygen output missing keys: %q", out.String())
	}

	sums := filepath.Join(t.TempDir(), "SHA256SUMS")
	content := []byte("0000000000000000000000000000000000000000000000000000000000000000  papeer-linux-amd64.tar.gz\n")
	os.WriteFile(sums, content, 0o644)

	t.Setenv("TEST_SIGNING_KEY", priv)
	if err := run([]string{"sign", "-key-env", "TEST_SIGNING_KEY", sums}, &out); err != nil {
		t.Fatalf("sign: %v", err)
	}
	sig, err := os.ReadFile(sums + ".sig")
	if err != nil {
		t.Fatal(err)
	}

	// The updater's verifier accepts it…
	if err := updater.VerifySignature(pub, content, sig); err != nil {
		t.Errorf("updater rejected signature: %v", err)
	}
	if err := run([]string{"verify", "-pub", pub, sums}, &out); err != nil {
		t.Errorf("verify: %v", err)
	}
	// …and rejects tampered content.
	if err := updater.VerifySignature(pub, append(content, 'x'), sig); err == nil {
		t.Error("tampered checksums accepted")
	}
}

func TestSignRequiresKey(t *testing.T) {
	sums := filepath.Join(t.TempDir(), "SHA256SUMS")
	os.WriteFile(sums, []byte("x"), 0o644)
	t.Setenv("EMPTY_KEY", "")
	if err := run([]string{"sign", "-key-env", "EMPTY_KEY", sums}, &bytes.Buffer{}); err == nil {
		t.Error("expected error for empty key")
	}
}
