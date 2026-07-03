package piplayer

import "testing"

func TestHashAndCheckHash(t *testing.T) {
	const password = "correct horse battery staple"

	h, err := hash(password)
	if err != nil {
		t.Fatalf("hash() returned error: %v", err)
	}
	if h == password {
		t.Error("hash() returned the plaintext password unchanged")
	}

	if !checkHash(password, h) {
		t.Error("checkHash() returned false for the correct password")
	}

	if checkHash("wrong password", h) {
		t.Error("checkHash() returned true for an incorrect password")
	}
}

func TestHashIsSalted(t *testing.T) {
	const password = "same password"

	h1, err := hash(password)
	if err != nil {
		t.Fatalf("hash() returned error: %v", err)
	}
	h2, err := hash(password)
	if err != nil {
		t.Fatalf("hash() returned error: %v", err)
	}

	if h1 == h2 {
		t.Error("two hashes of the same password are identical; expected bcrypt salt to differ")
	}

	// Both independently-salted hashes must still verify.
	if !checkHash(password, h1) || !checkHash(password, h2) {
		t.Error("checkHash() failed to verify a salted hash of the correct password")
	}
}
