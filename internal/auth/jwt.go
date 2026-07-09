package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Claims struct {
	UserID   string `json:"uid"`
	Username string `json:"un"`
	Role     string `json:"role"`
	Exp      int64  `json:"exp"`
	Iat      int64  `json:"iat"`
}

func IssueJWT(secret string, claims Claims) (string, error) {
	if secret == "" {
		return "", errors.New("jwt secret is empty")
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	body, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(body)
	sig := sign(secret, header+"."+payload)
	return header + "." + payload + "." + sig, nil
}

func ParseJWT(secret, token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid token")
	}
	want := sign(secret, parts[0]+"."+parts[1])
	if !hmac.Equal([]byte(want), []byte(parts[2])) {
		return nil, errors.New("invalid signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var claims Claims
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, err
	}
	if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
		return nil, errors.New("token expired")
	}
	return &claims, nil
}

func sign(secret, data string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func NewClaims(userID, username, role string, ttl time.Duration) Claims {
	now := time.Now()
	return Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		Iat:      now.Unix(),
		Exp:      now.Add(ttl).Unix(),
	}
}

func EnsureSecret(secret string) string {
	if strings.TrimSpace(secret) != "" {
		return secret
	}
	// Dev fallback; production should set jwtSecret.
	sum := sha256.Sum256([]byte(fmt.Sprintf("timenotes-blog-dev-%d", time.Now().UnixNano()/int64(time.Hour))))
	return base64.RawStdEncoding.EncodeToString(sum[:])
}
