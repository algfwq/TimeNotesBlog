package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/bits"
	"sync"
	"time"
)

// MaxPoWDifficultyBits keeps a browser-solvable proof of work bounded even if
// configuration is mistakenly set to an excessive value.
const MaxPoWDifficultyBits = 24

type Challenge struct {
	ID         string `json:"id"`
	Salt       string `json:"salt"`
	Difficulty int    `json:"difficulty"`
	ExpiresAt  int64  `json:"expiresAt"`
}

type powChallenge struct {
	Challenge
	ipHash    string
	wsSession string
}

type PoWManager struct {
	mu       sync.Mutex
	items    map[string]powChallenge
	base     int
	max      int
	lifetime time.Duration
}

func NewPoWManager(base, max int) *PoWManager {
	if base < 1 {
		base = 4
	}
	if base > MaxPoWDifficultyBits {
		base = MaxPoWDifficultyBits
	}
	if max < base {
		max = base
	}
	if max > MaxPoWDifficultyBits {
		max = MaxPoWDifficultyBits
	}
	return &PoWManager{
		items:    make(map[string]powChallenge),
		base:     base,
		max:      max,
		lifetime: 5 * time.Minute,
	}
}

func (m *PoWManager) DifficultyForFailures(failures int) int {
	d := m.base + failures/2
	if d > m.max {
		d = m.max
	}
	return d
}

// Issue is retained for local tooling. WebSocket authentication must use
// IssueBound so a proof cannot be replayed from another IP or connection.
func (m *PoWManager) Issue(failures int) (Challenge, error) {
	return m.IssueBound("", "", failures)
}

func (m *PoWManager) IssueBound(ipHash, wsSession string, failures int) (Challenge, error) {
	m.cleanup()
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return Challenge{}, err
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return Challenge{}, err
	}
	ch := Challenge{
		ID:         hex.EncodeToString(idBytes),
		Salt:       hex.EncodeToString(saltBytes),
		Difficulty: m.DifficultyForFailures(failures),
		ExpiresAt:  time.Now().Add(m.lifetime).Unix(),
	}
	m.mu.Lock()
	m.items[ch.ID] = powChallenge{Challenge: ch, ipHash: ipHash, wsSession: wsSession}
	m.mu.Unlock()
	return ch, nil
}

// Verify is retained for local tooling. WebSocket authentication must use
// VerifyBound so a proof cannot be replayed from another IP or connection.
func (m *PoWManager) Verify(id, nonce string) (bool, error) {
	return m.VerifyBound(id, "", "", nonce)
}

func (m *PoWManager) VerifyBound(id, ipHash, wsSession, nonce string) (bool, error) {
	m.mu.Lock()
	stored, ok := m.items[id]
	if ok {
		delete(m.items, id)
	}
	m.mu.Unlock()
	if !ok {
		return false, fmt.Errorf("challenge not found")
	}
	if time.Now().Unix() >= stored.ExpiresAt {
		return false, fmt.Errorf("challenge expired")
	}
	if stored.ipHash != ipHash || stored.wsSession != wsSession {
		return false, fmt.Errorf("challenge binding mismatch")
	}
// Binding is enforced by the challenge record above. The hash itself stays
		// salt+nonce so browsers and desktop clients do not need secret IP/session material.
		return validPoW(stored.Salt, nonce, stored.Difficulty), nil
	}

	func (m *PoWManager) cleanup() {
		now := time.Now().Unix()
		m.mu.Lock()
		defer m.mu.Unlock()
		for id, ch := range m.items {
			if now >= ch.ExpiresAt {
				delete(m.items, id)
			}
		}
	}

	func validPoW(salt, nonce string, difficulty int) bool {
		if difficulty < 1 || difficulty > MaxPoWDifficultyBits {
			return false
		}
		sum := sha256.Sum256([]byte(salt + nonce))
		return leadingZeroBits(sum[:]) >= difficulty
	}

func leadingZeroBits(value []byte) int {
	count := 0
	for _, b := range value {
		if b == 0 {
			count += 8
			continue
		}
		return count + bits.LeadingZeros8(uint8(b))
	}
	return count
}

// Solve is used by tests and local tools.
	func Solve(salt string, difficulty int) string {
		for i := 0; ; i++ {
			nonce := fmt.Sprintf("%d", i)
			if validPoW(salt, nonce, difficulty) {
				return nonce
			}
		}
	}

	// SolveBound is retained for older tests/tools; binding is checked separately by VerifyBound.
	func SolveBound(salt, ipHash, wsSession string, difficulty int) string {
		_ = ipHash
		_ = wsSession
		return Solve(salt, difficulty)
	}
