package config

import "testing"

func TestLoadProviderEnvDoesNotMaterializeDeployments(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Providers.OpenAI.APIKey != "sk-test" {
		t.Fatalf("providers.openai.api_key = %q, want env value", cfg.Providers.OpenAI.APIKey)
	}
	if len(cfg.Deployments) != 0 {
		t.Fatalf("deployments = %+v, want no implicit deployment config from provider env vars", cfg.Deployments)
	}
}
