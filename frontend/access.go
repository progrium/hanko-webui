package frontend

import "strings"

// AccessRule decides which Hanko users may use something (the admin UI or an OIDC client).
// A user is allowed if they match Users or satisfy the organization rule, or if AnyUser is set.
type AccessRule struct {
	// AnyUser allows every signed-in Hanko user. It must be set explicitly and can't be combined
	// with Users or Organization.
	AnyUser bool `yaml:"any_user"`
	// Users are user IDs, usernames, or verified emails (case-insensitive). Prefer IDs: a
	// username can be released and claimed by someone else.
	Users []string `yaml:"users"`
	// Organization is a Hanko organization ID (case-insensitive) or its exact name. Names are only
	// unique as exact strings in Hanko, so "Acme" doesn't match an organization named "acme".
	Organization string `yaml:"organization"`
	// Roles, if set, are role slugs of which the user must hold at least one in Organization.
	Roles []string `yaml:"roles"`
}

// Empty reports whether the rule allows nobody: no users, no organization, and not AnyUser.
func (a AccessRule) Empty() bool {
	return !a.AnyUser && len(a.users()) == 0 && strings.TrimSpace(a.Organization) == ""
}

func (a AccessRule) users() []string {
	var out []string
	for _, u := range a.Users {
		if u = strings.TrimSpace(u); u != "" {
			out = append(out, u)
		}
	}
	return out
}

// Allows reports whether the session's user satisfies the rule. An empty rule allows nobody.
func (a AccessRule) Allows(s *Session) bool {
	if s == nil {
		return false
	}
	if a.AnyUser {
		return true
	}
	if MatchUser(a.users(), s) {
		return true
	}
	org := a.Org(s)
	if org == nil {
		return false
	}
	if len(a.Roles) == 0 {
		return true
	}
	for _, want := range a.Roles {
		for _, have := range org.Roles {
			if strings.EqualFold(strings.TrimSpace(want), have) {
				return true
			}
		}
	}
	return false
}

// Org returns the session's membership in the rule's organization, or nil.
func (a AccessRule) Org(s *Session) *Organization {
	want := strings.TrimSpace(a.Organization)
	if s == nil || want == "" {
		return nil
	}
	for i := range s.Organizations {
		o := &s.Organizations[i]
		if strings.EqualFold(o.ID, want) || o.Name == want {
			return o
		}
	}
	return nil
}

// Validate checks that Roles are only used with Organization and AnyUser stands alone.
func (a AccessRule) Validate() error {
	if len(a.Roles) > 0 && strings.TrimSpace(a.Organization) == "" {
		return errRolesWithoutOrg
	}
	if a.AnyUser && (len(a.users()) > 0 || strings.TrimSpace(a.Organization) != "") {
		return errAnyUserCombined
	}
	return nil
}

// MatchUser reports whether the session's user matches any entry by user ID, username,
// or verified email (case-insensitive).
func MatchUser(entries []string, s *Session) bool {
	if s == nil {
		return false
	}
	for _, entry := range entries {
		switch {
		case strings.EqualFold(entry, s.UserID):
			return true
		case s.Username != "" && strings.EqualFold(entry, s.Username):
			return true
		case s.Email != "" && s.EmailVerified && strings.EqualFold(entry, s.Email):
			return true
		}
	}
	return false
}

type configError string

func (e configError) Error() string { return string(e) }

const (
	errRolesWithoutOrg = configError("roles require organization")
	errAnyUserCombined = configError("any_user can't be combined with users or organization")
)
