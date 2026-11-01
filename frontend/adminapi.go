package frontend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ErrNotFound is returned by AdminAPI when Hanko reports the resource doesn't exist.
var ErrNotFound = errors.New("not found")

// AdminAPI is a small client for the Hanko admin API, authenticated with the admin token.
type AdminAPI struct {
	base   *url.URL
	token  string
	client *http.Client
}

// NewAdminAPI returns a client for the admin API at base.
func NewAdminAPI(base *url.URL, token string) *AdminAPI {
	return &AdminAPI{base: base, token: token, client: &http.Client{Timeout: 10 * time.Second}}
}

// User is the subset of Hanko's admin user record that the frontend uses.
type User struct {
	ID     string `json:"id"`
	Emails []struct {
		Address    string `json:"address"`
		IsVerified bool   `json:"is_verified"`
		IsPrimary  bool   `json:"is_primary"`
	} `json:"emails"`
	Username *struct {
		Username string `json:"username"`
	} `json:"username"`
	Name       string `json:"name"`
	GivenName  string `json:"given_name"`
	FamilyName string `json:"family_name"`
	Picture    string `json:"picture"`
	Orgs       []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Roles []struct {
			Slug string `json:"slug"`
		} `json:"roles"`
	} `json:"organizations"`
}

// Organizations returns the user's organization memberships with role slugs.
func (u *User) Organizations() []Organization {
	out := make([]Organization, 0, len(u.Orgs))
	for _, o := range u.Orgs {
		org := Organization{ID: o.ID, Name: o.Name, Roles: []string{}}
		for _, r := range o.Roles {
			org.Roles = append(org.Roles, r.Slug)
		}
		out = append(out, org)
	}
	return out
}

// PrimaryEmail returns the primary email (or the first one) and whether it's verified.
func (u *User) PrimaryEmail() (string, bool) {
	for _, e := range u.Emails {
		if e.IsPrimary {
			return e.Address, e.IsVerified
		}
	}
	if len(u.Emails) > 0 {
		return u.Emails[0].Address, u.Emails[0].IsVerified
	}
	return "", false
}

// UserSession is an active Hanko session as listed by the admin API.
type UserSession struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	LastUsed  time.Time `json:"last_used"`
}

func (a *AdminAPI) do(ctx context.Context, method string, out any, path ...string) error {
	req, err := http.NewRequestWithContext(ctx, method, a.base.JoinPath(path...).String(), nil)
	if err != nil {
		return err
	}
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}
	req.Header.Set("Accept", "application/json")
	res, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case res.StatusCode >= 300:
		b, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return fmt.Errorf("admin API %s %s: %s %s", method, req.URL.Path, res.Status, b)
	case out != nil:
		return json.NewDecoder(res.Body).Decode(out)
	}
	return nil
}

// User fetches a user by ID.
func (a *AdminAPI) User(ctx context.Context, id string) (*User, error) {
	var u User
	if err := a.do(ctx, http.MethodGet, &u, "users", id); err != nil {
		return nil, err
	}
	return &u, nil
}

// Session returns the user's active session with the given ID, or nil if it no longer exists.
func (a *AdminAPI) Session(ctx context.Context, userID, sessionID string) (*UserSession, error) {
	var list []UserSession
	err := a.do(ctx, http.MethodGet, &list, "users", userID, "sessions")
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == sessionID {
			return &list[i], nil
		}
	}
	return nil, nil
}

// DeleteSession deletes one of the user's sessions. A missing session is not an error.
func (a *AdminAPI) DeleteSession(ctx context.Context, userID, sessionID string) error {
	err := a.do(ctx, http.MethodDelete, nil, "users", userID, "sessions", sessionID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
