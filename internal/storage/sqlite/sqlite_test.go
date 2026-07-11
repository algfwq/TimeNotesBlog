package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"timenotesblog/internal/storage"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "blog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func createTestUser(t *testing.T, store *Store, id, role string) storage.User {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	user := storage.User{ID: id, Username: id, PasswordHash: "hash", Role: role, CanUpload: true, CreatedAt: now, UpdatedAt: now}
	if err := store.CreateUser(context.Background(), user); err != nil {
		t.Fatal(err)
	}
	return user
}

func TestEnsureAdminRequiresCredentialChange(t *testing.T) {
	store := openTestStore(t)
	created, err := store.EnsureAdmin(context.Background(), "admin", "hash")
	if err != nil || !created {
		t.Fatalf("EnsureAdmin() = created=%v, err=%v", created, err)
	}
	user, err := store.GetUserByID(context.Background(), "admin")
	if err != nil {
		t.Fatal(err)
	}
	if !user.MustChangeCredentials {
		t.Fatal("default admin must require credential changes")
	}
}

func TestCreateNoteReservesCoverPathAndPublicDownload(t *testing.T) {
	store := openTestStore(t)
	owner := createTestUser(t, store, "owner", "user")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	note := storage.Note{
		ID: "note", OwnerUserID: owner.ID, Filename: "note.tnote", Title: "note", StoragePath: "stored.tnote", CoverPath: "thumbnail.png", SizeBytes: 1, SHA256: "sha", Visible: true, PublicDownload: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateNote(context.Background(), note); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetNote(context.Background(), note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CoverPath != note.CoverPath || !got.PublicDownload {
		t.Fatalf("note fields not persisted: %+v", got)
	}
}

func TestDeleteUserTransfersNotesAtomicallyAndKeepsAnAdmin(t *testing.T) {
	store := openTestStore(t)
	adminOne := createTestUser(t, store, "admin-one", "admin")
	adminTwo := createTestUser(t, store, "admin-two", "admin")
	owner := createTestUser(t, store, "owner", "user")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := store.CreateNote(context.Background(), storage.Note{ID: "note", OwnerUserID: owner.ID, Filename: "note.tnote", Title: "note", StoragePath: "stored.tnote", SizeBytes: 1, SHA256: "sha", Visible: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteUserAndTransferNotes(context.Background(), owner.ID, adminOne.ID); err != nil {
		t.Fatal(err)
	}
	note, err := store.GetNote(context.Background(), "note")
	if err != nil || note.OwnerUserID != adminOne.ID {
		t.Fatalf("note owner after transfer = %+v, err=%v", note, err)
	}
	if err := store.DeleteUserAndTransferNotes(context.Background(), adminOne.ID, adminTwo.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteUserAndTransferNotes(context.Background(), adminTwo.ID, ""); !errors.Is(err, storage.ErrLastAdmin) {
		t.Fatalf("deleting last admin error = %v, want ErrLastAdmin", err)
	}
	count, err := store.CountAdmins(context.Background())
	if err != nil || count != 1 {
		t.Fatalf("admin count = %d, err=%v", count, err)
	}
}

func TestConsumeDownloadTokenIsOneTime(t *testing.T) {
	store := openTestStore(t)
	owner := createTestUser(t, store, "owner", "user")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := store.CreateNote(context.Background(), storage.Note{ID: "note", OwnerUserID: owner.ID, Filename: "note.tnote", Title: "note", StoragePath: "stored.tnote", SizeBytes: 1, SHA256: "sha", Visible: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateDownloadToken(context.Background(), "token", "note", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	noteID, err := store.ConsumeDownloadToken(context.Background(), "token")
	if err != nil || noteID != "note" {
		t.Fatalf("first consume = %q, %v", noteID, err)
	}
	if _, err := store.ConsumeDownloadToken(context.Background(), "token"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("second consume error = %v, want ErrNotFound", err)
	}
}
