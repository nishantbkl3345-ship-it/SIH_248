package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"fogline/api/internal/domain"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func TestPasswordHashAndVerify(t *testing.T) {
	h := NewPasswordHasher(bcrypt.MinCost)
	hash, err := h.Hash("correct horse battery")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if hash == "correct horse battery" {
		t.Fatal("hash equals the plaintext")
	}
	if !h.Verify(hash, "correct horse battery") {
		t.Error("Verify rejected the right password")
	}
	if h.Verify(hash, "wrong password") {
		t.Error("Verify accepted the wrong password")
	}
	if h.Verify("not-a-hash", "correct horse battery") {
		t.Error("Verify accepted a malformed hash")
	}
}

func TestPasswordHashRejectsOverlongInput(t *testing.T) {
	h := NewPasswordHasher(bcrypt.MinCost)
	if _, err := h.Hash(strings.Repeat("a", MaxPasswordBytes+1)); err == nil {
		t.Fatal("expected an error for a 73-byte password")
	}
}

func TestTokenRoundTrip(t *testing.T) {
	m := NewTokenManager(testSecret, "fogline", time.Hour)
	id := uuid.New()

	token, exp, err := m.Issue(id, domain.RoleInstructor)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if time.Until(exp) < 59*time.Minute {
		t.Errorf("expiry too soon: %v", exp)
	}

	claims, err := m.Parse(token)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got, _ := claims.UserID()
	if got != id || claims.Role != domain.RoleInstructor {
		t.Errorf("claims = %+v", claims)
	}
}

func TestTokenRejected(t *testing.T) {
	m := NewTokenManager(testSecret, "fogline", time.Hour)
	id := uuid.New()
	valid, _, _ := m.Issue(id, domain.RoleTrainee)

	expired := NewTokenManager(testSecret, "fogline", time.Hour)
	expired.now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	expiredToken, _, _ := expired.Issue(id, domain.RoleTrainee)

	otherKey, _, _ := NewTokenManager(strings.Repeat("x", 32), "fogline", time.Hour).Issue(id, domain.RoleTrainee)
	otherIssuer, _, _ := NewTokenManager(testSecret, "someone-else", time.Hour).Issue(id, domain.RoleTrainee)
	badRole, _, _ := m.Issue(id, domain.Role("SUPERUSER"))

	// alg=none must never be accepted.
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, Claims{
		Role: domain.RoleAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "fogline",
			Subject:   id.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("building unsigned token: %v", err)
	}

	cases := map[string]string{
		"empty":        "",
		"garbage":      "not.a.token",
		"tampered":     valid[:len(valid)-2] + "xx",
		"expired":      expiredToken,
		"other key":    otherKey,
		"other issuer": otherIssuer,
		"unknown role": badRole,
		"alg none":     unsigned,
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := m.Parse(token); !errors.Is(err, ErrInvalidToken) {
				t.Errorf("Parse error = %v, want ErrInvalidToken", err)
			}
		})
	}
}
