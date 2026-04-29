package cryptobox

import (
	"bytes"
	"testing"
)

var testMagic = Magic{'C', 'C', 'T', 'X'}

// Use lower-effort params in tests so they don't take seconds each.
var fastParams = Params{Time: 1, MemMB: 8, Threads: 1}

func TestSealOpen_RoundTrip(t *testing.T) {
	plaintext := []byte("hello, secret world")
	pass := []byte("correct horse battery staple")

	sealed, err := Seal(testMagic, pass, plaintext, fastParams)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(sealed, plaintext) {
		t.Fatalf("plaintext leaked into ciphertext")
	}

	got, err := Open(&testMagic, pass, sealed)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("round-trip mismatch: %q", got)
	}
}

func TestOpen_WrongPassphrase(t *testing.T) {
	sealed, err := Seal(testMagic, []byte("right"), []byte("data"), fastParams)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(&testMagic, []byte("wrong"), sealed); err != ErrAuth {
		t.Fatalf("expected ErrAuth, got %v", err)
	}
}

func TestOpen_TamperedCiphertext(t *testing.T) {
	sealed, err := Seal(testMagic, []byte("pw"), []byte("data"), fastParams)
	if err != nil {
		t.Fatal(err)
	}
	// Flip one byte in the ciphertext region (after header).
	sealed[headerLen+1] ^= 0xff
	if _, err := Open(&testMagic, []byte("pw"), sealed); err != ErrAuth {
		t.Fatalf("expected ErrAuth on tamper, got %v", err)
	}
}

func TestOpen_TamperedHeader(t *testing.T) {
	sealed, err := Seal(testMagic, []byte("pw"), []byte("data"), fastParams)
	if err != nil {
		t.Fatal(err)
	}
	// Flip a byte inside the salt — header is bound into auth tag.
	sealed[10] ^= 0x01
	if _, err := Open(&testMagic, []byte("pw"), sealed); err != ErrAuth {
		t.Fatalf("expected ErrAuth on header tamper, got %v", err)
	}
}

func TestOpen_BadMagic(t *testing.T) {
	sealed, err := Seal(testMagic, []byte("pw"), []byte("data"), fastParams)
	if err != nil {
		t.Fatal(err)
	}
	other := Magic{'X', 'X', 'X', 'X'}
	if _, err := Open(&other, []byte("pw"), sealed); err != ErrFormat {
		t.Fatalf("expected ErrFormat on magic mismatch, got %v", err)
	}
}

func TestOpen_BadVersion(t *testing.T) {
	sealed, err := Seal(testMagic, []byte("pw"), []byte("data"), fastParams)
	if err != nil {
		t.Fatal(err)
	}
	sealed[4] = 0x99
	if _, err := Open(&testMagic, []byte("pw"), sealed); err != ErrFormat {
		t.Fatalf("expected ErrFormat on version mismatch, got %v", err)
	}
}

func TestOpen_TooShort(t *testing.T) {
	if _, err := Open(&testMagic, []byte("pw"), []byte("short")); err != ErrFormat {
		t.Fatalf("expected ErrFormat on short payload, got %v", err)
	}
}

func TestSeal_ParamsRoundTrip(t *testing.T) {
	// Custom params should be readable back via Open without specifying them.
	custom := Params{Time: 3, MemMB: 4, Threads: 2}
	sealed, err := Seal(testMagic, []byte("pw"), []byte("x"), custom)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Open(&testMagic, []byte("pw"), sealed)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if string(got) != "x" {
		t.Fatalf("got %q", got)
	}
}
