package auth

import "testing"

func TestTokenRoundTrip(t *testing.T) {
	token, hash, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	got, ok := TokenHash(token)
	if !ok || got != hash {
		t.Fatal("token hash mismatch")
	}
	if _, ok := TokenHash(token + "x"); ok {
		t.Fatal("accepted malformed token")
	}
	other, _, err := NewToken()
	if err != nil || other == token {
		t.Fatal("token collision or error")
	}
}

func TestArgon2id(t *testing.T) {
	encoded, err := HashPassword("long test password 123")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(encoded, "long test password 123") {
		t.Fatal("correct password rejected")
	}
	if VerifyPassword(encoded, "wrong password") {
		t.Fatal("wrong password accepted")
	}
	if VerifyPassword("$argon2id$v=19$m=999999999,t=3,p=4$bad$bad", "anything") {
		t.Fatal("unsafe hash accepted")
	}
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("short password accepted")
	}
}
