package password

import "testing"

const strongPassword = "correct horse battery staple"

func TestHasherHashAndVerify(t *testing.T) {
	hasher := NewHasher()

	encoded, err := hasher.Hash(strongPassword)
	if err != nil {
		t.Fatalf("Hash() error = %v", err)
	}

	valid, err := hasher.Verify(strongPassword, encoded)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !valid {
		t.Fatal("Verify() = false, want true")
	}

	valid, err = hasher.Verify("another password value", encoded)
	if err != nil {
		t.Fatalf("Verify() wrong password error = %v", err)
	}
	if valid {
		t.Fatal("Verify() wrong password = true, want false")
	}
}

func TestHasherUsesUniqueSalt(t *testing.T) {
	hasher := NewHasher()

	first, err := hasher.Hash(strongPassword)
	if err != nil {
		t.Fatalf("first Hash() error = %v", err)
	}
	second, err := hasher.Hash(strongPassword)
	if err != nil {
		t.Fatalf("second Hash() error = %v", err)
	}

	if first == second {
		t.Fatal("two hashes are equal; salt was not unique")
	}
}

func TestHasherDummyHashIsValidAndDoesNotMatch(t *testing.T) {
	hasher := NewHasher()

	valid, err := hasher.Verify("untrusted password value", hasher.DummyHash())
	if err != nil {
		t.Fatalf("Verify() dummy hash error = %v", err)
	}
	if valid {
		t.Fatal("untrusted password matched dummy hash")
	}
}

func TestValidateRejectsShortPassword(t *testing.T) {
	if err := Validate("too short"); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}
