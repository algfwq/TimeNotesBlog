package auth

import (
	"testing"
	"time"
)

func TestPasswordAndJWT(t *testing.T) {
	hash, err := HashPassword("123456", "pepper")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyPassword(hash, "123456", "pepper")
	if err != nil || !ok {
		t.Fatalf("verify failed: %v %v", ok, err)
	}
	ok, _ = VerifyPassword(hash, "wrong", "pepper")
	if ok {
		t.Fatal("expected mismatch")
	}

	claims := NewClaims("u1", "admin", "admin", time.Hour)
	token, err := IssueJWT("secret-key-123456", claims)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseJWT("secret-key-123456", token)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Username != "admin" || parsed.Role != "admin" {
		t.Fatalf("unexpected claims: %+v", parsed)
	}
}

func TestPoW(t *testing.T) {
	m := NewPoWManager(2, 8)
	ch, err := m.Issue(0)
	if err != nil {
		t.Fatal(err)
	}
	nonce := Solve(ch.Salt, ch.Difficulty)
	ok, err := m.Verify(ch.ID, nonce)
	if err != nil || !ok {
		t.Fatalf("pow verify failed: %v %v", ok, err)
	}
}
