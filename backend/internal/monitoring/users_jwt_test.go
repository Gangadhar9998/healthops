package monitoring

import "testing"

func TestInitJWTSecretFromEnvRequiresSecret(t *testing.T) {
	t.Setenv("HEALTHOPS_JWT_SECRET", "")

	if err := InitJWTSecretFromEnv(); err == nil {
		t.Fatal("InitJWTSecretFromEnv() error = nil, want missing secret error")
	}
}

func TestInitJWTSecretFromEnvRejectsShortSecret(t *testing.T) {
	t.Setenv("HEALTHOPS_JWT_SECRET", "too-short")

	if err := InitJWTSecretFromEnv(); err == nil {
		t.Fatal("InitJWTSecretFromEnv() error = nil, want short secret error")
	}
}

func TestInitJWTSecretFromEnvSignsAndVerifiesTokens(t *testing.T) {
	t.Setenv("HEALTHOPS_JWT_SECRET", "12345678901234567890123456789012")

	if err := InitJWTSecretFromEnv(); err != nil {
		t.Fatalf("InitJWTSecretFromEnv() error = %v", err)
	}

	token, err := signJWT(JWTClaims{
		UserID:   "u1",
		Username: "admin",
		Role:     RoleAdmin,
		Exp:      4102444800,
		Iat:      1700000000,
	})
	if err != nil {
		t.Fatalf("signJWT() error = %v", err)
	}

	claims, err := verifyJWT(token)
	if err != nil {
		t.Fatalf("verifyJWT() error = %v", err)
	}
	if claims.Username != "admin" || claims.Role != RoleAdmin {
		t.Fatalf("claims = %+v, want admin role", claims)
	}
}
