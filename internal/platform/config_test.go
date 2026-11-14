package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestModeAndDatabaseMustBeExplicitAndMatch(t *testing.T) {
	for _, tc := range []struct{ mode, db string }{{"", "shop_demo"}, {"production", "shop_live"}, {"demo", "shop_live"}, {"live", "shop_demo"}} {
		t.Run(tc.mode+tc.db, func(t *testing.T) {
			t.Setenv("APP_MODE", tc.mode)
			t.Setenv("POSTGRES_DB", tc.db)
			if _, err := LoadConfig(); err == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
}

func TestServerConfigurationRequiresCredentials(t *testing.T) {
	t.Setenv("APP_MODE", "demo")
	t.Setenv("POSTGRES_DB", "shop_demo")
	t.Setenv("POSTGRES_HOST", "postgres")
	t.Setenv("POSTGRES_USER", "shop")
	t.Setenv("POSTGRES_PASSWORD_FILE", "")
	t.Setenv("POSTGRES_PASSWORD", "")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("missing password accepted")
	}
	t.Setenv("POSTGRES_PASSWORD", "test-only")
	if _, err := LoadConfig(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_MODE", "live")
	t.Setenv("POSTGRES_DB", "shop_live")
	if _, err := LoadConfig(); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(t.TempDir(), "db-password")
	if err := os.WriteFile(secret, []byte("file-test-only\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POSTGRES_PASSWORD_FILE", secret)
	config, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.DB.ConnConfig.Password != "file-test-only" {
		t.Fatal("secret file not used")
	}
	t.Setenv("POSTGRES_PASSWORD_FILE", secret+"-missing")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("missing secret file accepted")
	}
}
