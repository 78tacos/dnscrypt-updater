package apply

import (
	"fmt"
	"io"
	"os"

	"aead.dev/minisign"
	"github.com/78tacos/dnscrypt-updater/internal/githubrel"
)

func verifyArchive(archivePath string, sig []byte, pubKey string) error {
	if pubKey == "" {
		pubKey = githubrel.MinisignPubKey
	}
	var pk minisign.PublicKey
	if err := pk.UnmarshalText([]byte(pubKey)); err != nil {
		return fmt.Errorf("minisign public key: %w", err)
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	r := minisign.NewReader(f)
	if _, err := io.Copy(io.Discard, r); err != nil {
		return fmt.Errorf("read archive for minisign: %w", err)
	}
	if r.Verify(pk, sig) {
		return nil
	}

	raw, err := os.ReadFile(archivePath)
	if err != nil {
		return err
	}
	if minisign.Verify(pk, raw, sig) {
		return nil
	}
	return fmt.Errorf("minisign verification failed for %s (pubkey %s)", archivePath, pubKey)
}
