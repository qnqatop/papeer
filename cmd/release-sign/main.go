// Command release-sign manages the ed25519 key that signs release checksums
// for the auto-updater (see internal/updater/verify.go).
//
//	release-sign keygen
//	    Print a new key pair as UPDATE_SIGNING_KEY=<base64 private key> and
//	    UPDATE_PUBLIC_KEY=<base64 public key>.
//	release-sign sign -key-env UPDATE_SIGNING_KEY SHA256SUMS
//	    Sign the file with the private key read from the named environment
//	    variable and write the base64 signature to SHA256SUMS.sig.
//	release-sign verify -pub <base64 public key> SHA256SUMS
//	    Check SHA256SUMS.sig against the public key (sanity check in CI).
package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/qnqatop/papeer/internal/updater"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "release-sign:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: release-sign keygen | sign -key-env VAR FILE | verify -pub KEY FILE")
	}
	switch args[0] {
	case "keygen":
		pub, priv, err := ed25519.GenerateKey(nil)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "UPDATE_SIGNING_KEY=%s\n", base64.StdEncoding.EncodeToString(priv))
		fmt.Fprintf(stdout, "UPDATE_PUBLIC_KEY=%s\n", base64.StdEncoding.EncodeToString(pub))
		return nil

	case "sign":
		fs := flag.NewFlagSet("sign", flag.ContinueOnError)
		keyEnv := fs.String("key-env", "UPDATE_SIGNING_KEY", "environment variable holding the base64 private key")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: release-sign sign -key-env VAR FILE")
		}
		priv, err := decodePrivateKey(os.Getenv(*keyEnv))
		if err != nil {
			return fmt.Errorf("%s: %w", *keyEnv, err)
		}
		msg, err := os.ReadFile(fs.Arg(0))
		if err != nil {
			return err
		}
		sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, msg)) + "\n"
		return os.WriteFile(fs.Arg(0)+".sig", []byte(sig), 0o644)

	case "verify":
		fs := flag.NewFlagSet("verify", flag.ContinueOnError)
		pub := fs.String("pub", "", "base64 public key")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 || *pub == "" {
			return errors.New("usage: release-sign verify -pub KEY FILE")
		}
		msg, err := os.ReadFile(fs.Arg(0))
		if err != nil {
			return err
		}
		sig, err := os.ReadFile(fs.Arg(0) + ".sig")
		if err != nil {
			return err
		}
		if err := updater.VerifySignature(*pub, msg, sig); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "signature OK")
		return nil
	}
	return fmt.Errorf("unknown command %q", args[0])
}

// decodePrivateKey accepts a base64 ed25519 private key (64 bytes) or seed
// (32 bytes).
func decodePrivateKey(b64 string) (ed25519.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, fmt.Errorf("decode private key: %w", err)
	}
	switch len(raw) {
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(raw), nil
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(raw), nil
	}
	return nil, fmt.Errorf("private key must be %d or %d bytes, got %d", ed25519.PrivateKeySize, ed25519.SeedSize, len(raw))
}
