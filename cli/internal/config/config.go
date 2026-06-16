// Copyright (C) 2026 Gerald Hofbauer <info@geraldhofbauer.net>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds the persisted CLI session state.
// Encrypted private keys are stored; master password is never persisted.
type Config struct {
	ServerURL       string `mapstructure:"server_url"`
	UserID          string `mapstructure:"user_id"`
	Email           string `mapstructure:"email"`
	RefreshToken    string `mapstructure:"refresh_token"`
	PubX25519       string `mapstructure:"pub_x25519"`       // base64
	PubMLKEM768     string `mapstructure:"pub_mlkem768"`     // base64
	EncPrivX25519   string `mapstructure:"enc_priv_x25519"`  // base64 (AES-GCM encrypted)
	EncPrivMLKEM768 string `mapstructure:"enc_priv_mlkem768"` // base64 (AES-GCM encrypted)
	KDFSalt         string `mapstructure:"kdf_salt"`         // base64
	KDFTime         int    `mapstructure:"kdf_time"`
	KDFMemory       int    `mapstructure:"kdf_memory"`
}

// IsLoggedIn returns true if a valid session exists.
func (c *Config) IsLoggedIn() bool {
	return c.ServerURL != "" && c.RefreshToken != "" && c.UserID != ""
}

// ConfigPath returns the path to the config file.
func ConfigPath(override string) string {
	if override != "" {
		return override
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "pwmgr", "config.yaml")
}

// Load reads the config from disk. Returns empty config if file doesn't exist.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// Save writes the config to disk (mode 0600).
func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	v.Set("server_url", c.ServerURL)
	v.Set("user_id", c.UserID)
	v.Set("email", c.Email)
	v.Set("refresh_token", c.RefreshToken)
	v.Set("pub_x25519", c.PubX25519)
	v.Set("pub_mlkem768", c.PubMLKEM768)
	v.Set("enc_priv_x25519", c.EncPrivX25519)
	v.Set("enc_priv_mlkem768", c.EncPrivMLKEM768)
	v.Set("kdf_salt", c.KDFSalt)
	v.Set("kdf_time", c.KDFTime)
	v.Set("kdf_memory", c.KDFMemory)

	if err := v.WriteConfigAs(path); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return os.Chmod(path, 0600)
}

// Clear wipes session data (logout).
func (c *Config) Clear() {
	c.RefreshToken = ""
	c.UserID = ""
	c.Email = ""
	c.EncPrivX25519 = ""
	c.EncPrivMLKEM768 = ""
	c.KDFSalt = ""
	c.KDFTime = 0
	c.KDFMemory = 0
}
