package merchant

import (
	"context"
	"database/sql"
	"testing"

	"epay/ent"
	"epay/ent/enttest"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newTestClient(t *testing.T) *ent.Client {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(drv)))
	t.Cleanup(func() {
		_ = client.Close()
		_ = db.Close()
	})
	return client
}

func TestCreateSelfRegisteredMerchantUsesPasswordForLoginAndKeepsPkeyAsAPIKey(t *testing.T) {
	ctx := context.Background()
	svc := NewService(newTestClient(t))

	registered, err := svc.CreateSelfRegisteredMerchant(ctx, "Self Serve Shop", "correct horse battery staple", 0.006, "")
	if err != nil {
		t.Fatalf("CreateSelfRegisteredMerchant returned error: %v", err)
	}
	if registered.Pid == 0 {
		t.Fatal("registered merchant pid was not assigned")
	}
	if registered.Pkey == "" {
		t.Fatal("registered merchant pkey was not returned for EasyPay API signing")
	}
	if registered.Pkey == "correct horse battery staple" {
		t.Fatal("pkey must remain a generated API key, not the login password")
	}

	loggedIn, err := svc.VerifyCredential(ctx, registered.Pid, "correct horse battery staple")
	if err != nil {
		t.Fatalf("VerifyCredential rejected registration password: %v", err)
	}
	if loggedIn.ID != registered.ID {
		t.Fatalf("logged in merchant id = %s, want %s", loggedIn.ID, registered.ID)
	}

	if _, err := svc.VerifyCredential(ctx, registered.Pid, "wrong password"); err == nil {
		t.Fatal("VerifyCredential accepted an invalid password")
	}
}

func TestAdminCreatedMerchantKeepsLegacyPkeyLogin(t *testing.T) {
	ctx := context.Background()
	svc := NewService(newTestClient(t))

	created, err := svc.CreateMerchant(ctx, "Admin Created Shop", 0.01, "")
	if err != nil {
		t.Fatalf("CreateMerchant returned error: %v", err)
	}

	if _, err := svc.VerifyCredential(ctx, created.Pid, created.Pkey); err != nil {
		t.Fatalf("VerifyCredential rejected legacy pkey login: %v", err)
	}
}
