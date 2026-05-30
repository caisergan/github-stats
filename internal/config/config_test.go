package config

import "testing"

func TestLoadRequiresClientID(t *testing.T) {
	env := map[string]string{
		"GITHUB_CLIENT_SECRET": "s",
		"ENCRYPTION_KEY":       "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
	}
	_, err := Load(func(k string) string { return env[k] })
	if err == nil {
		t.Fatal("expected error when GITHUB_CLIENT_ID missing")
	}
}

func TestLoadParsesDefaultsAndKey(t *testing.T) {
	env := map[string]string{
		"GITHUB_CLIENT_ID":     "id",
		"GITHUB_CLIENT_SECRET": "secret",
		"ENCRYPTION_KEY":       "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
	}
	cfg, err := Load(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Addr default = %q, want :8080", cfg.Addr)
	}
	if cfg.DatabasePath != "app.db" {
		t.Errorf("DatabasePath default = %q, want app.db", cfg.DatabasePath)
	}
	if len(cfg.EncryptionKey) != 32 {
		t.Errorf("EncryptionKey len = %d, want 32", len(cfg.EncryptionKey))
	}
	if cfg.RedirectURL() != "http://localhost:8080/auth/github/callback" {
		t.Errorf("RedirectURL = %q", cfg.RedirectURL())
	}
}

func TestLoadRejectsBadKey(t *testing.T) {
	env := map[string]string{
		"GITHUB_CLIENT_ID":     "id",
		"GITHUB_CLIENT_SECRET": "secret",
		"ENCRYPTION_KEY":       "tooshort",
	}
	if _, err := Load(func(k string) string { return env[k] }); err == nil {
		t.Fatal("expected error for non-32-byte key")
	}
}
