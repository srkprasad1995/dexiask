package auth

import "testing"

func TestIndexerAuthHeaders(t *testing.T) {
	// Gating disabled when no internal token.
	if h := IndexerAuthHeaders("", true, "tok"); h != nil {
		t.Fatalf("expected nil when no internal token, got %+v", h)
	}
	// Admin → unrestricted (never leaks the user token).
	admin := IndexerAuthHeaders("secret", true, "gho_admin")
	if admin["X-Internal-Token"] != "secret" || admin["X-Repo-Access"] != "all" {
		t.Fatalf("admin headers = %+v", admin)
	}
	if _, leaked := admin["X-User-Token"]; leaked {
		t.Fatalf("admin must not forward a user token: %+v", admin)
	}
	// Member → forwards their token for the indexer to validate; not unrestricted.
	member := IndexerAuthHeaders("secret", false, "gho_member")
	if member["X-Internal-Token"] != "secret" || member["X-User-Token"] != "gho_member" {
		t.Fatalf("member headers = %+v", member)
	}
	if _, unrestricted := member["X-Repo-Access"]; unrestricted {
		t.Fatalf("member must not be unrestricted: %+v", member)
	}
}
