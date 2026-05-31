package crypto

import (
	"bytes"
	"testing"
)

func key32() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i)
	}
	return k
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c, err := NewCipher(key32())
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("gho_secrettoken")
	enc, err := c.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	if enc == string(plain) {
		t.Fatal("ciphertext equals plaintext")
	}
	got, err := c.Decrypt(enc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("round trip mismatch: %q != %q", got, plain)
	}
}

func TestEncryptIsNonDeterministic(t *testing.T) {
	c, _ := NewCipher(key32())
	a, _ := c.Encrypt([]byte("same"))
	b, _ := c.Encrypt([]byte("same"))
	if a == b {
		t.Fatal("expected unique ciphertext per call (random nonce)")
	}
}

func TestDecryptRejectsTampered(t *testing.T) {
	c, _ := NewCipher(key32())
	enc, _ := c.Encrypt([]byte("data"))
	tampered := "00" + enc[2:]
	if _, err := c.Decrypt(tampered); err == nil {
		t.Fatal("expected error on tampered ciphertext")
	}
}

func TestNewCipherRejectsBadKey(t *testing.T) {
	if _, err := NewCipher(make([]byte, 16)); err == nil {
		t.Fatal("expected error for non-32-byte key")
	}
}
