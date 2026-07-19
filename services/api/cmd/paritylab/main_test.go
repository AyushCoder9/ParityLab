package main

import (
	"testing"
)

func TestValidateSandboxKeyRejectsEveryLivePrefix(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"sk_live_secret", "rk_live_restricted", "pk_live_public", "  sk_live_space"} {
		if err := validateSandboxKey(key); err == nil {
			t.Fatalf("accepted live key prefix: %q", key)
		}
	}
	for _, key := range []string{"", "sk_test_secret", "rk_test_restricted", "pk_test_public"} {
		if err := validateSandboxKey(key); err != nil {
			t.Fatalf("rejected sandbox key %q: %v", key, err)
		}
	}
}

func TestStripeAPIBaseRequiresExplicitMockMode(t *testing.T) {
	t.Setenv("STRIPE_API_BASE", "http://stripe-mock:12111")
	if _, err := configuredStripeAPIBase(); err == nil {
		t.Fatal("accepted API override without explicit mock mode")
	}
	t.Setenv("PARITYLAB_ALLOW_STRIPE_MOCK", "true")
	if got, err := configuredStripeAPIBase(); err != nil || got != "http://stripe-mock:12111" {
		t.Fatalf("base=%q err=%v", got, err)
	}
}
