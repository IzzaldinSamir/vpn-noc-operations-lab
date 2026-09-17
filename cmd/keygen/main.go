package main

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

type keyPair struct {
	private string
	public  string
}

func generateKeyPair() (keyPair, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return keyPair{}, fmt.Errorf("generate private key: %w", err)
	}

	// WireGuard private keys are X25519 scalars in clamped form.
	raw[0] &= 248
	raw[31] &= 127
	raw[31] |= 64

	private, err := ecdh.X25519().NewPrivateKey(raw)
	if err != nil {
		return keyPair{}, fmt.Errorf("create X25519 key: %w", err)
	}

	return keyPair{
		private: base64.StdEncoding.EncodeToString(private.Bytes()),
		public:  base64.StdEncoding.EncodeToString(private.PublicKey().Bytes()),
	}, nil
}

func config(address, privateKey, peerPublicKey, peerEndpoint, peerAddress string) string {
	return fmt.Sprintf(`[Interface]
Address = %s/30
PrivateKey = %s
ListenPort = 51820
MTU = 1420

[Peer]
PublicKey = %s
AllowedIPs = %s/32
Endpoint = %s:51820
PersistentKeepalive = 5
`, address, privateKey, peerPublicKey, peerAddress, peerEndpoint)
}

func writeConfig(path, contents string, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists; use --force to rotate lab keys", path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(contents), 0o600)
}

func run(outDir string, force bool) error {
	siteA, err := generateKeyPair()
	if err != nil {
		return err
	}
	siteB, err := generateKeyPair()
	if err != nil {
		return err
	}

	aPath := filepath.Join(outDir, "site-a", "wg0.conf")
	bPath := filepath.Join(outDir, "site-b", "wg0.conf")
	if err := writeConfig(aPath, config("10.77.0.1", siteA.private, siteB.public, "172.30.0.3", "10.77.0.2"), force); err != nil {
		return err
	}
	if err := writeConfig(bPath, config("10.77.0.2", siteB.private, siteA.public, "172.30.0.2", "10.77.0.1"), force); err != nil {
		return err
	}

	fmt.Printf("Generated ephemeral lab configuration in %s\n", outDir)
	fmt.Printf("site-a public key: %s\n", siteA.public)
	fmt.Printf("site-b public key: %s\n", siteB.public)
	return nil
}

func main() {
	outDir := flag.String("out", "runtime/wireguard", "output directory")
	force := flag.Bool("force", false, "replace existing lab keys")
	flag.Parse()

	if err := run(*outDir, *force); err != nil {
		fmt.Fprintln(os.Stderr, "key generation failed:", err)
		os.Exit(1)
	}
}
