package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
)

func main() {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "generate Ed25519 key pair: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf(
		"IDENTITY_ACCESS_TOKEN_PRIVATE_KEY_BASE64=%s\nIDENTITY_ACCESS_TOKEN_PUBLIC_KEY_BASE64=%s\n",
		base64.StdEncoding.EncodeToString(privateKey),
		base64.StdEncoding.EncodeToString(publicKey),
	)
}
