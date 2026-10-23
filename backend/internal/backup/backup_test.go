package backup_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"filippo.io/age"

	"ttacommunity/backend/internal/backup"
)

func newIdentity(t *testing.T) *age.X25519Identity {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRecipientFromRefusesAMissingOrWrongKey(t *testing.T) {
	tests := []struct {
		name, value string
	}{
		{"no recipient", ""},
		{"a recipient that is not an age key", "age1notakey"},
		{"a private key as the recipient", newIdentity(t).String()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(k string) string {
				if k == "BACKUP_AGE_RECIPIENT" {
					return tt.value
				}
				return ""
			}
			if _, err := backup.RecipientFrom(getenv); err == nil {
				t.Errorf("RecipientFrom(%q): no error, want a refusal", tt.value)
			}
		})
	}
}

func TestRecipientFromReadsThePublicKey(t *testing.T) {
	id := newIdentity(t)
	r, err := backup.RecipientFrom(func(k string) string {
		if k == "BACKUP_AGE_RECIPIENT" {
			return id.Recipient().String()
		}
		return ""
	})
	if err != nil {
		t.Fatalf("RecipientFrom: %v", err)
	}
	if r.String() != id.Recipient().String() {
		t.Errorf("recipient = %s, want %s", r, id.Recipient())
	}
}

// at is when the tests' backups are taken.
var at = time.Date(2026, 10, 21, 3, 0, 5, 0, time.UTC)

func leftIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestWritePutsOneFileEncryptedToTheRecipient(t *testing.T) {
	id := newIdentity(t)
	dir := t.TempDir()
	path, size, err := backup.Write(dir, backup.Name(at), id.Recipient(), func(w io.Writer) error {
		_, err := io.WriteString(w, "a backup")
		return err
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if want := filepath.Join(dir, "2026-10-21T030005Z.zip.age"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if left := leftIn(t, dir); len(left) != 1 {
		t.Errorf("files in the folder = %q, want the backup alone", left)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || info.Size() != size {
		t.Errorf("the file's size = %v (%v), Write returned %d", info.Size(), err, size)
	}
	r, err := age.Decrypt(f, id)
	if err != nil {
		t.Fatalf("decrypting the file: %v", err)
	}
	if got, _ := io.ReadAll(r); string(got) != "a backup" {
		t.Errorf("decrypted = %q, want %q", got, "a backup")
	}
}

func TestWriteLeavesNothingWhenTheBackupFails(t *testing.T) {
	failed := errors.New("the database went away")
	dir := t.TempDir()
	_, _, err := backup.Write(dir, backup.Name(at), newIdentity(t).Recipient(), func(w io.Writer) error {
		io.WriteString(w, "half a backup")
		return failed
	})
	if !errors.Is(err, failed) {
		t.Errorf("Write = %v, want the backup's error", err)
	}
	if left := leftIn(t, dir); len(left) != 0 {
		t.Errorf("Write left %q in its folder", left)
	}
}

func TestWriteRefusesAFolderThatIsNotThere(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	if _, _, err := backup.Write(dir, backup.Name(at), newIdentity(t).Recipient(), func(io.Writer) error { return nil }); err == nil {
		t.Error("Write to a missing folder: no error")
	}
}

func TestNameNamesTheBackupByItsTimeInUTC(t *testing.T) {
	sydney, err := time.LoadLocation("Australia/Sydney")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{"in UTC", time.Date(2026, 10, 21, 3, 0, 5, 0, time.UTC), "2026-10-21T030005Z.zip.age"},
		{"in another zone", time.Date(2026, 10, 21, 14, 0, 5, 0, sydney), "2026-10-21T030005Z.zip.age"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := backup.Name(tt.at); got != tt.want {
				t.Errorf("Name(%v) = %q, want %q", tt.at, got, tt.want)
			}
		})
	}
}
