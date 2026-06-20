package llmimport

import "testing"

func TestParseModelsTOMLParsesOneProfileIntoOneAccount(t *testing.T) {
	raw := []byte(`
[deepseek-v4-flash]
host = "cloud"
model_name = "deepseek-v4-flash"
api_key = "sk-deepseek"
base_url = "https://api.deepseek.com"
timeout_sec = 200
thinking_type = "enabled"
max_inflight = 100
max_requests_per_minute = 300
max_tokens_per_minute = 500000
token_reserve_per_call = 256
`)

	got, err := ParseModelsTOML(raw)
	if err != nil {
		t.Fatalf("ParseModelsTOML() error = %v", err)
	}
	if len(got.Accounts) != 1 {
		t.Fatalf("accounts = %d, want 1", len(got.Accounts))
	}
	if len(got.Profiles) != 1 {
		t.Fatalf("profiles = %d, want 1", len(got.Profiles))
	}

	account := got.Accounts[0]
	if account.Provider != "deepseek" {
		t.Fatalf("provider = %q, want deepseek", account.Provider)
	}
	if account.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("base_url = %q", account.BaseURL)
	}

	profile := got.Profiles[0]
	if profile.ProfileName != "deepseek-v4-flash" {
		t.Fatalf("profile_name = %q", profile.ProfileName)
	}
	if profile.ModelName != "deepseek-v4-flash" {
		t.Fatalf("model_name = %q", profile.ModelName)
	}
	if profile.ThinkingType != "enabled" || profile.TimeoutSec != 200 {
		t.Fatalf("unexpected profile values: %+v", profile)
	}
}

func TestParseModelsTOMLDeduplicatesProfilesSharingSameAccount(t *testing.T) {
	raw := []byte(`
[deepseek-v4-flash]
host = "cloud"
model_name = "deepseek-v4-flash"
api_key = "sk-deepseek"
base_url = "https://api.deepseek.com"
timeout_sec = 200

[deepseek-v4-pro]
host = "cloud"
model_name = "deepseek-v4-pro"
api_key = "sk-deepseek"
base_url = "https://api.deepseek.com"
timeout_sec = 300
`)

	got, err := ParseModelsTOML(raw)
	if err != nil {
		t.Fatalf("ParseModelsTOML() error = %v", err)
	}
	if len(got.Accounts) != 1 {
		t.Fatalf("accounts = %d, want 1", len(got.Accounts))
	}
	if len(got.Profiles) != 2 {
		t.Fatalf("profiles = %d, want 2", len(got.Profiles))
	}
	if got.Profiles[0].AccountKey != got.Profiles[1].AccountKey {
		t.Fatalf("expected shared account key, got %q and %q", got.Profiles[0].AccountKey, got.Profiles[1].AccountKey)
	}
}

func TestParseModelsTOMLMakesAccountNamesUniqueForMultipleAccountsOnSameHost(t *testing.T) {
	raw := []byte(`
[deepseek-a]
host = "cloud"
model_name = "deepseek-chat"
api_key = "sk-deepseek-a"
base_url = "https://api.deepseek.com"

[deepseek-b]
host = "cloud"
model_name = "deepseek-reasoner"
api_key = "sk-deepseek-b"
base_url = "https://api.deepseek.com"
`)

	got, err := ParseModelsTOML(raw)
	if err != nil {
		t.Fatalf("ParseModelsTOML() error = %v", err)
	}
	if len(got.Accounts) != 2 {
		t.Fatalf("accounts = %d, want 2", len(got.Accounts))
	}
	if got.Accounts[0].Name == got.Accounts[1].Name {
		t.Fatalf("expected distinct account names, got %q and %q", got.Accounts[0].Name, got.Accounts[1].Name)
	}
}
