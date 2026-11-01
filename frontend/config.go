package frontend

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultConfigFile matches Hanko's default config path.
const DefaultConfigFile = "./config/config.yaml"

// ServerConfig holds the frontend's settings from the "server" section of a Hanko config.
// server.frontend is our addition; Hanko ignores unknown keys.
type ServerConfig struct {
	Public   string
	Admin    string
	Frontend string
	// Admins is server.frontend.admins: user IDs, usernames, or verified emails allowed into /admin.
	Admins []string
}

// DefaultServerConfig returns Hanko's default public/admin addresses plus the frontend default.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{Public: ":8000", Admin: ":8001", Frontend: "127.0.0.1:8080"}
}

type serverSettings struct {
	Address string `yaml:"address"`
}

type configFile struct {
	Server struct {
		Public   serverSettings `yaml:"public"`
		Admin    serverSettings `yaml:"admin"`
		Frontend struct {
			Address string   `yaml:"address"`
			Admins  []string `yaml:"admins"`
		} `yaml:"frontend"`
	} `yaml:"server"`
}

// LoadServerConfig reads server addresses from a Hanko config file, falling back to defaults.
// Like Hanko, a missing file is only an error when it isn't the default path, and the
// SERVER_PUBLIC_ADDRESS, SERVER_ADMIN_ADDRESS, and SERVER_FRONTEND_ADDRESS environment
// variables override the file, as does SERVER_FRONTEND_ADMINS (comma-separated).
func LoadServerConfig(path string) (ServerConfig, error) {
	cfg := DefaultServerConfig()
	if path == "" {
		path = DefaultConfigFile
	}
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		var f configFile
		if err := yaml.Unmarshal(b, &f); err != nil {
			return cfg, fmt.Errorf("parse %s: %w", path, err)
		}
		set(&cfg.Public, f.Server.Public.Address)
		set(&cfg.Admin, f.Server.Admin.Address)
		set(&cfg.Frontend, f.Server.Frontend.Address)
		cfg.Admins = f.Server.Frontend.Admins
	case errors.Is(err, fs.ErrNotExist) && path == DefaultConfigFile:
	default:
		return cfg, fmt.Errorf("load config: %w", err)
	}
	set(&cfg.Public, os.Getenv("SERVER_PUBLIC_ADDRESS"))
	set(&cfg.Admin, os.Getenv("SERVER_ADMIN_ADDRESS"))
	set(&cfg.Frontend, os.Getenv("SERVER_FRONTEND_ADDRESS"))
	if v := os.Getenv("SERVER_FRONTEND_ADMINS"); v != "" {
		cfg.Admins = strings.Split(v, ",")
	}
	return cfg, nil
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
