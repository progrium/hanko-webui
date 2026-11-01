package oidc

import (
	"context"
	"net/http"

	"github.com/progrium/hankoid/frontend"
)

// Hanko is what the provider needs from Hanko. The frontend's Auth and AdminAPI implement it.
type Hanko interface {
	// Session validates the request's Hanko session cookie; nil means not signed in.
	Session(r *http.Request) (*frontend.Session, error)
	// Logout ends the request's Hanko session, writing any cookie-clearing headers to w.
	Logout(w http.ResponseWriter, r *http.Request) error
	// User fetches a user's profile.
	User(ctx context.Context, userID string) (*frontend.User, error)
	// UserSession returns the user's session, or nil if it no longer exists.
	UserSession(ctx context.Context, userID, sessionID string) (*frontend.UserSession, error)
	// DeleteSession ends a session by ID.
	DeleteSession(ctx context.Context, userID, sessionID string) error
}

// NewHanko adapts the frontend's session validator and admin API client.
func NewHanko(auth *frontend.Auth, admin *frontend.AdminAPI) Hanko {
	return hanko{auth, admin}
}

type hanko struct {
	auth  *frontend.Auth
	admin *frontend.AdminAPI
}

func (h hanko) Session(r *http.Request) (*frontend.Session, error)  { return h.auth.Session(r) }
func (h hanko) Logout(w http.ResponseWriter, r *http.Request) error { return h.auth.Logout(w, r) }
func (h hanko) User(ctx context.Context, id string) (*frontend.User, error) {
	return h.admin.User(ctx, id)
}
func (h hanko) UserSession(ctx context.Context, userID, sessionID string) (*frontend.UserSession, error) {
	return h.admin.Session(ctx, userID, sessionID)
}
func (h hanko) DeleteSession(ctx context.Context, userID, sessionID string) error {
	return h.admin.DeleteSession(ctx, userID, sessionID)
}
