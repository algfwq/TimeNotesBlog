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

	claims := NewClaims("u1", "admin", "admin", 0, time.Hour)
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

func TestPoWIsBoundToIPAndWebSocketSessionAndSingleUse(t *testing.T) {
	m := NewPoWManager(4, 8)
	challenge, err := m.IssueBound("ip-hash-a", "ws-session-a", 0)
	if err != nil {
		t.Fatal(err)
	}
	if challenge.Difficulty < 4 || challenge.Difficulty > 8 {
		t.Fatalf("difficulty outside configured bit bounds: %d", challenge.Difficulty)
	}

	nonce := SolveBound(challenge.Salt, "ip-hash-a", "ws-session-a", challenge.Difficulty)
	ok, err := m.VerifyBound(challenge.ID, "ip-hash-a", "ws-session-b", nonce)
	if err == nil || ok {
		t.Fatalf("challenge must reject a different websocket session: ok=%v err=%v", ok, err)
	}
	ok, err = m.VerifyBound(challenge.ID, "ip-hash-a", "ws-session-a", nonce)
	if err == nil || ok {
		t.Fatalf("failed verification must consume challenge: ok=%v err=%v", ok, err)
	}
}

func TestPoWExpiresAndCleansBoundChallenges(t *testing.T) {
	m := NewPoWManager(1, 4)
	m.lifetime = -time.Second
	challenge, err := m.IssueBound("ip-hash", "ws-session", 0)
	if err != nil {
		t.Fatal(err)
	}
	m.cleanup()
	if len(m.items) != 0 {
		t.Fatalf("expired challenge %s was not cleaned", challenge.ID)
	}
}

func TestPoWMaxDifficultyIsHardBounded(t *testing.T) {
	m := NewPoWManager(1, 100)
	challenge, err := m.IssueBound("ip-hash", "ws-session", 1_000)
	if err != nil {
		t.Fatal(err)
	}
	if challenge.Difficulty > MaxPoWDifficultyBits {
		t.Fatalf("difficulty %d exceeds hard cap %d", challenge.Difficulty, MaxPoWDifficultyBits)
	}
}
