package main

import (
	"strings"
	"testing"
)

func TestRequiredEnvRejectsMissingValues(t *testing.T) {
	t.Setenv("HEALTHOPS_TEST_REQUIRED", "")

	_, err := requiredEnv("HEALTHOPS_TEST_REQUIRED")
	if err == nil {
		t.Fatal("expected missing env to fail")
	}
	if !strings.Contains(err.Error(), "HEALTHOPS_TEST_REQUIRED") {
		t.Fatalf("expected error to name env var, got %q", err.Error())
	}
}

func TestRequiredEnvReturnsConfiguredValue(t *testing.T) {
	t.Setenv("HEALTHOPS_TEST_REQUIRED", "configured")

	value, err := requiredEnv("HEALTHOPS_TEST_REQUIRED")
	if err != nil {
		t.Fatalf("requiredEnv returned error: %v", err)
	}
	if value != "configured" {
		t.Fatalf("expected configured value, got %q", value)
	}
}

func TestRequiredSecretRejectsShortValues(t *testing.T) {
	t.Setenv("HEALTHOPS_TEST_SECRET", "too-short")

	_, err := requiredSecret("HEALTHOPS_TEST_SECRET", 32)
	if err == nil {
		t.Fatal("expected short secret to fail")
	}
	if !strings.Contains(err.Error(), "at least 32 bytes") {
		t.Fatalf("expected length error, got %q", err.Error())
	}
}

func TestRequiredSecretReturnsConfiguredValue(t *testing.T) {
	secret := strings.Repeat("a", 32)
	t.Setenv("HEALTHOPS_TEST_SECRET", secret)

	value, err := requiredSecret("HEALTHOPS_TEST_SECRET", 32)
	if err != nil {
		t.Fatalf("requiredSecret returned error: %v", err)
	}
	if value != secret {
		t.Fatalf("expected configured secret, got %q", value)
	}
}
