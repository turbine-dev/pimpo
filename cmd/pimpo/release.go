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
)

// releaseKeys are the public keys a release's checksums.txt must be signed
// with. More than one allows a new key to ship before the old one retires.
// Tests replace them.
var releaseKeys = []string{"Axmu5Iq5+WmaiZin5Dr77QoOGVFD0No5cu2sBGUNBGQ="}

const releaseUsage = `usage:
  pimpo release sign --key-env NAME | --key KEYFILE [--out SIGFILE] FILE   (maintainers: writes FILE.sig, the base64 Ed25519 signature of FILE)`

func releaseCmd(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "sign" {
		return errors.New(releaseUsage)
	}
	fs := flag.NewFlagSet("release sign", flag.ContinueOnError)
	keyEnv := fs.String("key-env", "", "environment variable holding the private key")
	keyFile := fs.String("key", "", "private key file")
	outFile := fs.String("out", "", "where to write the signature (default FILE.sig)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 1 || (*keyEnv == "") == (*keyFile == "") {
		return errors.New(releaseUsage)
	}
	priv := ""
	if *keyEnv != "" {
		if priv = os.Getenv(*keyEnv); priv == "" {
			return fmt.Errorf("%s is empty; it must hold the release signing key", *keyEnv)
		}
	} else {
		raw, err := os.ReadFile(*keyFile)
		if err != nil {
			return err
		}
		priv = string(raw)
	}
	data, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return err
	}
	sig, err := signBytes(strings.TrimSpace(priv), data)
	if err != nil {
		return err
	}
	dest := *outFile
	if dest == "" {
		dest = fs.Arg(0) + ".sig"
	}
	if err := os.WriteFile(dest, []byte(sig+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "signed %s into %s\n", fs.Arg(0), dest)
	return nil
}

// signBytes signs data with a base64 Ed25519 private key and returns the
// base64 signature.
func signBytes(private string, data []byte) (string, error) {
	key, err := base64.StdEncoding.DecodeString(private)
	if err != nil || len(key) != ed25519.PrivateKeySize {
		return "", errors.New("not an Ed25519 private key")
	}
	return base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(key), data)), nil
}

// signedByOneOf reports whether sig (base64, surrounding space ignored) is
// a signature of data by any of keys.
func signedByOneOf(keys []string, data []byte, sig string) bool {
	s, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sig))
	if err != nil || len(s) != ed25519.SignatureSize {
		return false
	}
	for _, k := range keys {
		pub, err := base64.StdEncoding.DecodeString(k)
		if err == nil && len(pub) == ed25519.PublicKeySize && ed25519.Verify(ed25519.PublicKey(pub), data, s) {
			return true
		}
	}
	return false
}
