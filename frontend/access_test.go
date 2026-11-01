package frontend

import "testing"

func TestAccessRule(t *testing.T) {
	alice := &Session{UserID: "u1", Email: "Alice@X.io", EmailVerified: true}
	bob := &Session{UserID: "u2", Username: "bob", Organizations: []Organization{{ID: "o1", Name: "Listmonk", Roles: []string{"editor"}}}}
	eve := &Session{UserID: "u3", Email: "eve@x.io", EmailVerified: false}
	cases := []struct {
		name string
		rule AccessRule
		s    *Session
		want bool
	}{
		{"empty denies", AccessRule{}, alice, false},
		{"nil session", AccessRule{Users: []string{"u1"}}, nil, false},
		{"by id", AccessRule{Users: []string{"U1"}}, alice, true},
		{"by verified email", AccessRule{Users: []string{"alice@x.io"}}, alice, true},
		{"unverified email denied", AccessRule{Users: []string{"eve@x.io"}}, eve, false},
		{"by username", AccessRule{Users: []string{" bob "}}, bob, true},
		{"org by exact name", AccessRule{Organization: "Listmonk"}, bob, true},
		{"org name is case-sensitive", AccessRule{Organization: "listmonk"}, bob, false},
		{"org id is case-insensitive", AccessRule{Organization: "O1"}, bob, true},
		{"any user", AccessRule{AnyUser: true}, eve, true},
		{"any user needs a session", AccessRule{AnyUser: true}, nil, false},
		{"org by id", AccessRule{Organization: "o1"}, bob, true},
		{"org role match", AccessRule{Organization: "Listmonk", Roles: []string{"admin", "Editor"}}, bob, true},
		{"org role miss", AccessRule{Organization: "Listmonk", Roles: []string{"admin"}}, bob, false},
		{"not member", AccessRule{Organization: "Listmonk"}, alice, false},
		{"either rule", AccessRule{Users: []string{"alice@x.io"}, Organization: "Listmonk"}, alice, true},
	}
	for _, c := range cases {
		if got := c.rule.Allows(c.s); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
	if (AccessRule{Roles: []string{"x"}}).Validate() == nil {
		t.Error("roles without organization should be invalid")
	}
	if (AccessRule{AnyUser: true, Users: []string{"a"}}).Validate() == nil {
		t.Error("any_user combined with users should be invalid")
	}
}
