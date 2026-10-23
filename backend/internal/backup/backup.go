// Package backup writes a backup of the ratings database, encrypted with age, as one file for the backup job to
// upload.
package backup

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"filippo.io/age"
)

// RecipientFrom reads BACKUP_AGE_RECIPIENT with getenv: the public key backups are encrypted to. It refuses a
// missing setting and one that is not an age X25519 public key.
func RecipientFrom(getenv func(string) string) (*age.X25519Recipient, error) {
	v := getenv("BACKUP_AGE_RECIPIENT")
	if v == "" {
		return nil, errors.New("BACKUP_AGE_RECIPIENT is not set")
	}
	r, err := age.ParseX25519Recipient(v)
	if err != nil {
		return nil, errors.New("BACKUP_AGE_RECIPIENT is not an age public key (age1…)")
	}
	return r, nil
}

// Write writes what write produces, encrypted to r, to dir as name, and returns the file's path and size.
// The file gets its name only once complete, so a failed backup leaves nothing in dir. Nothing unencrypted reaches
// the disk.
func Write(dir, name string, r age.Recipient, write func(io.Writer) error) (string, int64, error) {
	// The partial file's name never ends in .age, so the upload cannot take it.
	f, err := os.CreateTemp(dir, "."+name+".partial-*")
	if err != nil {
		return "", 0, fmt.Errorf("backing up: %w", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	enc, err := age.Encrypt(f, r)
	if err != nil {
		return "", 0, fmt.Errorf("backing up: encrypting: %w", err)
	}
	if err := write(enc); err != nil {
		return "", 0, err
	}
	if err := enc.Close(); err != nil {
		return "", 0, fmt.Errorf("backing up: encrypting: %w", err)
	}
	if err := f.Sync(); err != nil {
		return "", 0, fmt.Errorf("backing up: %w", err)
	}
	size, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return "", 0, fmt.Errorf("backing up: %w", err)
	}
	path := filepath.Join(dir, name)
	if err := os.Rename(f.Name(), path); err != nil {
		return "", 0, fmt.Errorf("backing up: %w", err)
	}
	return path, size, nil
}

// Name names the database backup taken at at, in UTC: 2026-10-21T030005Z.zip.age.
func Name(at time.Time) string {
	return Stamp(at) + ".zip.age"
}

// Stamp is the time a backup was taken, in UTC, as its file names start: 2026-10-21T030005Z.
func Stamp(at time.Time) string {
	return at.UTC().Format("2006-01-02T150405Z")
}
