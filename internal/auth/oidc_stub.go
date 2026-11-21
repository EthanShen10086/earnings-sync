package auth

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

// MintStubOIDCToken builds a JWT-shaped bearer token for local AUTH_MODE=oidc.
// Signature is not cryptographically verified in stub mode — only claim shape + iss/aud.
func MintStubOIDCToken(claims map[string]any) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, _ := json.Marshal(claims)
	body := base64.RawURLEncoding.EncodeToString(payload)
	sig := base64.RawURLEncoding.EncodeToString([]byte("stub-sig"))
	return header + "." + body + "." + sig
}

type oidcClaims struct {
	Sub   string   `json:"sub"`
	Email string   `json:"email"`
	Name  string   `json:"name"`
	Iss   string   `json:"iss"`
	Aud   any      `json:"aud"` // string or []string
	Roles []string `json:"roles"`
}

func parseStubOIDC(token string) (oidcClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return oidcClaims{}, errAuth("oidc stub requires JWT-shaped bearer (header.payload.sig)")
	}
	for i, p := range parts {
		if strings.TrimSpace(p) == "" {
			return oidcClaims{}, errAuth("oidc stub JWT segment empty")
		}
		if i < 2 {
			if _, err := base64.RawURLEncoding.DecodeString(p); err != nil {
				// also accept std encoding
				if _, err2 := base64.URLEncoding.DecodeString(p); err2 != nil {
					return oidcClaims{}, errAuth("oidc stub JWT segment not base64url")
				}
			}
		}
	}
	raw, err := decodeB64(parts[1])
	if err != nil {
		return oidcClaims{}, errAuth("oidc stub payload decode failed")
	}
	var c oidcClaims
	if err := json.Unmarshal(raw, &c); err != nil {
		return oidcClaims{}, errAuth("oidc stub payload must be JSON claims")
	}
	if strings.TrimSpace(c.Sub) == "" {
		return oidcClaims{}, errAuth("oidc stub claims.sub required")
	}
	return c, nil
}

func (c oidcClaims) audienceOK(want string) bool {
	if want == "" {
		return true
	}
	switch v := c.Aud.(type) {
	case string:
		return v == want
	case []any:
		for _, a := range v {
			if s, ok := a.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

func decodeB64(s string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return base64.URLEncoding.DecodeString(s)
}
