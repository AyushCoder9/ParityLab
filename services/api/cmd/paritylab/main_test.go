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

func TestInsecureCookiesRequireLoopbackHTTPOrigin(t *testing.T) {
	t.Setenv("PARITYLAB_INSECURE_COOKIES", "true")
	for _, origin := range []string{
		"http://127.0.0.1:3000",
		"http://localhost:3000",
		"http://[::1]:3000",
	} {
		if insecure, err := configuredCookiePolicy(origin); err != nil || !insecure {
			t.Fatalf("origin=%q insecure=%v err=%v", origin, insecure, err)
		}
	}
	for _, origin := range []string{
		"https://127.0.0.1:3000",
		"http://example.com",
		"https://example.com",
		"not-a-url",
	} {
		if insecure, err := configuredCookiePolicy(origin); err == nil || insecure {
			t.Fatalf("accepted non-loopback origin=%q insecure=%v err=%v", origin, insecure, err)
		}
	}
	t.Setenv("PARITYLAB_INSECURE_COOKIES", "false")
	if insecure, err := configuredCookiePolicy("https://paritylab.example"); err != nil || insecure {
		t.Fatalf("secure default insecure=%v err=%v", insecure, err)
	}
}
