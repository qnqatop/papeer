package updater

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// PublicKey is the base64-encoded ed25519 public key that release checksum
// files must be signed with. It is injected at build time:
//
//	go build -ldflags "-X github.com/qnqatop/papeer/internal/updater.PublicKey=<base64>"
//
// Empty (dev builds, or before the maintainer has configured a key) means the
// signature check is skipped — see verifyChecksums for the exact policy.
var PublicKey string

const (
	checksumsAsset = "SHA256SUMS"
	signatureAsset = "SHA256SUMS.sig"
)

// DecodePublicKey parses a base64-encoded ed25519 public key.
func DecodePublicKey(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, fmt.Errorf("decode public key: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public key must be %d bytes, got %d", ed25519.PublicKeySize, len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

// VerifySignature checks that sigB64 (a base64 raw ed25519 signature, as
// stored in SHA256SUMS.sig) is a valid signature of msg under the base64
// public key pubB64. Shared by the updater and cmd/release-sign.
func VerifySignature(pubB64 string, msg, sigB64 []byte) error {
	pub, err := DecodePublicKey(pubB64)
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(sigB64)))
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	if len(sig) != ed25519.SignatureSize || !ed25519.Verify(pub, msg, sig) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

// parseChecksums parses a sha256sum-style file ("<hex>  <filename>" per line,
// optionally "<hex> *<filename>" for binary mode) into filename → digest.
// Conflicting duplicate entries are rejected.
func parseChecksums(data []byte) (map[string][]byte, error) {
	sums := make(map[string][]byte)
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		hexSum, name, ok := strings.Cut(line, " ")
		if !ok {
			return nil, fmt.Errorf("malformed checksum line: %q", line)
		}
		name = strings.TrimPrefix(strings.TrimLeft(name, " "), "*")
		sum, err := hex.DecodeString(hexSum)
		if err != nil || len(sum) != 32 || name == "" {
			return nil, fmt.Errorf("malformed checksum line: %q", line)
		}
		if prev, dup := sums[name]; dup && !bytes.Equal(prev, sum) {
			return nil, fmt.Errorf("conflicting checksums for %s", name)
		}
		sums[name] = sum
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read checksums: %w", err)
	}
	return sums, nil
}
