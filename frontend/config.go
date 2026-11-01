package frontend

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultConfigFile matches Hanko's default config path.
const DefaultConfigFile = "./config/config.yaml"

// Config holds the frontend's settings from a Hanko config file. server.frontend is our
// addition; Hanko ignores unknown keys.
type Config struct {
	Public   string
	Admin    string
	Frontend string
	// PublicURL is server.frontend.public_url: the frontend's externally visible origin.
	PublicURL string
	// AdminAccess is server.frontend.admin_access, merged with the legacy server.frontend.admins.
	AdminAccess AccessRule
	// SessionSameSite is session.cookie.same_site from the Hanko config ("strict" by default).
	SessionSameSite string
	// SessionSecure is session.cookie.secure from the Hanko config (true by default).
	SessionSecure bool
	// TrustedProxies is server.frontend.trusted_proxies: IPs/CIDRs of reverse proxies whose
	// X-Forwarded-For is used to find the client IP.
	TrustedProxies []string
	// Raw is the config file's contents, for other packages that read their own keys from it.
	Raw []byte
	// Dir is the config file's directory, for resolving relative paths in it.
	Dir string
}

// DefaultConfig returns Hanko's default public/admin addresses plus the frontend default.
func DefaultConfig() Config {
	return Config{Public: ":8000", Admin: ":8001", Frontend: "127.0.0.1:8080", SessionSameSite: "strict", SessionSecure: true}
}

type serverSettings struct {
	Address string `yaml:"address"`
}

type configFile struct {
	Server struct {
		Public   serverSettings `yaml:"public"`
		Admin    serverSettings `yaml:"admin"`
		Frontend struct {
			Address        string     `yaml:"address"`
			PublicURL      string     `yaml:"public_url"`
			Admins         []string   `yaml:"admins"`
			AdminAccess    AccessRule `yaml:"admin_access"`
			TrustedProxies []string   `yaml:"trusted_proxies"`
		} `yaml:"frontend"`
	} `yaml:"server"`
	Session struct {
		Cookie struct {
			SameSite string `yaml:"same_site"`
			Secure   *bool  `yaml:"secure"`
		} `yaml:"cookie"`
	} `yaml:"session"`
}

// ReadConfigFile reads a Hanko config file. Like Hanko, a missing file is only an error when
// it isn't the default path; then nil is returned.
func ReadConfigFile(path string) ([]byte, error) {
	if path == "" {
		path = DefaultConfigFile
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) && path == DefaultConfigFile {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	return b, nil
}

// LoadConfig reads the frontend's settings from a Hanko config file, falling back to defaults.
// Environment variables override the file: SERVER_PUBLIC_ADDRESS, SERVER_ADMIN_ADDRESS,
// SERVER_FRONTEND_ADDRESS, SERVER_FRONTEND_PUBLIC_URL, SERVER_FRONTEND_ADMINS (comma-separated,
// alias for admin_access.users), SERVER_FRONTEND_ADMIN_ACCESS_ORGANIZATION,
// SERVER_FRONTEND_ADMIN_ACCESS_ROLES (comma-separated), SERVER_FRONTEND_TRUSTED_PROXIES
// (comma-separated), and SESSION_COOKIE_SAME_SITE.
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()
	b, err := ReadConfigFile(path)
	if err != nil {
		return cfg, err
	}
	cfg.Raw = b
	if path == "" {
		path = DefaultConfigFile
	}
	cfg.Dir = filepath.Dir(path)
	if b != nil {
		var f configFile
		if err := yaml.Unmarshal(b, &f); err != nil {
			return cfg, fmt.Errorf("parse %s: %w", path, err)
		}
		fe := f.Server.Frontend
		set(&cfg.Public, f.Server.Public.Address)
		set(&cfg.Admin, f.Server.Admin.Address)
		set(&cfg.Frontend, fe.Address)
		set(&cfg.PublicURL, fe.PublicURL)
		set(&cfg.SessionSameSite, f.Session.Cookie.SameSite)
		if f.Session.Cookie.Secure != nil {
			cfg.SessionSecure = *f.Session.Cookie.Secure
		}
		cfg.AdminAccess = fe.AdminAccess
		cfg.TrustedProxies = fe.TrustedProxies
		cfg.AdminAccess.Users = append(cfg.AdminAccess.Users, fe.Admins...)
	}
	set(&cfg.Public, os.Getenv("SERVER_PUBLIC_ADDRESS"))
	set(&cfg.Admin, os.Getenv("SERVER_ADMIN_ADDRESS"))
	set(&cfg.Frontend, os.Getenv("SERVER_FRONTEND_ADDRESS"))
	set(&cfg.PublicURL, os.Getenv("SERVER_FRONTEND_PUBLIC_URL"))
	set(&cfg.SessionSameSite, os.Getenv("SESSION_COOKIE_SAME_SITE"))
	if v := os.Getenv("SESSION_COOKIE_SECURE"); v != "" {
		cfg.SessionSecure = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("SERVER_FRONTEND_TRUSTED_PROXIES"); v != "" {
		cfg.TrustedProxies = SplitList(v)
	}
	if v := os.Getenv("SERVER_FRONTEND_ADMINS"); v != "" {
		cfg.AdminAccess.Users = SplitList(v)
	}
	set(&cfg.AdminAccess.Organization, os.Getenv("SERVER_FRONTEND_ADMIN_ACCESS_ORGANIZATION"))
	if v := os.Getenv("SERVER_FRONTEND_ADMIN_ACCESS_ROLES"); v != "" {
		cfg.AdminAccess.Roles = SplitList(v)
	}
	if cfg.PublicURL == "" {
		cfg.PublicURL = defaultPublicURL(cfg.Frontend)
	}
	cfg.PublicURL = strings.TrimRight(cfg.PublicURL, "/")
	return cfg, cfg.Validate()
}

// Validate checks the frontend settings.
func (c Config) Validate() error {
	if err := ValidatePublicURL(c.PublicURL); err != nil {
		return fmt.Errorf("server.frontend.public_url: %w", err)
	}
	if err := c.AdminAccess.Validate(); err != nil {
		return fmt.Errorf("server.frontend.admin_access: %w", err)
	}
	if _, err := ParseProxies(c.TrustedProxies); err != nil {
		return fmt.Errorf("server.frontend.trusted_proxies: %w", err)
	}
	if c.AdminAccess.AnyUser {
		return errors.New("server.frontend.admin_access: any_user isn't allowed for /admin")
	}
	return nil
}

// Warnings reports risky but allowed settings.
func (c Config) Warnings() []string {
	var out []string
	if host, _, err := net.SplitHostPort(c.Admin); err == nil && !isLoopback(host) {
		out = append(out, fmt.Sprintf("server.admin.address %q listens beyond loopback; Hanko's admin API has no authentication of its own, so anyone who can reach it controls all users. Use 127.0.0.1:<port> unless a gateway enforces auth.", c.Admin))
	}
	if strings.HasPrefix(c.PublicURL, "https://") && !c.SessionSecure {
		out = append(out, "server.frontend.public_url is https but session.cookie.secure is false; session cookies would also be sent over plain http")
	}
	if strings.EqualFold(c.SessionSameSite, "none") {
		out = append(out, "session.cookie.same_site is none; session cookies are sent on cross-site requests, weakening CSRF protection")
	}
	return out
}

// ValidatePublicURL requires an absolute http(s) origin without path, query, or fragment.
// Plain http is only allowed for localhost and loopback addresses.
func ValidatePublicURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("%q must be an absolute http(s) URL", raw)
	}
	if u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return fmt.Errorf("%q must be an origin without path, query, fragment, or credentials", raw)
	}
	if u.Scheme == "http" && !isLoopback(u.Hostname()) {
		return fmt.Errorf("%q must use https (http is only allowed for localhost)", raw)
	}
	return nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// defaultPublicURL derives http://localhost:<port> from a listen address.
func defaultPublicURL(listen string) string {
	_, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "http://localhost"
	}
	return "http://" + net.JoinHostPort("localhost", port)
}

// SplitList splits a comma-separated list, trimming spaces and dropping empty entries.
func SplitList(v string) []string {
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func set(dst *string, v string) {
	if v != "" {
		*dst = v
	}
}

// AddrURL turns a listen address like ":8001" into a dialable URL like "http://127.0.0.1:8001".
func AddrURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + addr
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}
