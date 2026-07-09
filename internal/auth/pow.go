package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Challenge struct {
	ID         string `json:"id"`
	Salt       string `json:"salt"`
	Difficulty int    `json:"difficulty"`
	ExpiresAt  int64  `json:"expiresAt"`
}

type PoWManager struct {
	mu       sync.Mutex
	items    map[string]Challenge
	base     int
	max      int
	lifetime time.Duration
}

func NewPoWManager(base, max int) *PoWManager {
	if base < 1 {
		base = 4
	}
	if max < base {
		max = base
	}
	return &PoWManager{
		items:    make(map[string]Challenge),
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

func (m *PoWManager) Issue(failures int) (Challenge, error) {
	m.cleanup()
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return Challenge{}, err
	}
	idBytes := make([]byte, 8)
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
	m.items[ch.ID] = ch
	m.mu.Unlock()
	return ch, nil
}

func (m *PoWManager) Verify(id, nonce string) (bool, error) {
	m.mu.Lock()
	ch, ok := m.items[id]
	if ok {
		delete(m.items, id)
	}
	m.mu.Unlock()
	if !ok {
		return false, fmt.Errorf("challenge not found")
	}
	if time.Now().Unix() > ch.ExpiresAt {
		return false, fmt.Errorf("challenge expired")
	}
	sum := sha256.Sum256([]byte(ch.Salt + nonce))
	hexSum := hex.EncodeToString(sum[:])
	prefix := strings.Repeat("0", ch.Difficulty)
	return strings.HasPrefix(hexSum, prefix), nil
}

func (m *PoWManager) cleanup() {
	now := time.Now().Unix()
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, ch := range m.items {
		if now > ch.ExpiresAt {
			delete(m.items, id)
		}
	}
}

// Solve is used by tests and local tools.
func Solve(salt string, difficulty int) string {
	prefix := strings.Repeat("0", difficulty)
	for i := 0; ; i++ {
		nonce := fmt.Sprintf("%d", i)
		sum := sha256.Sum256([]byte(salt + nonce))
		if strings.HasPrefix(hex.EncodeToString(sum[:]), prefix) {
			return nonce
		}
	}
}
