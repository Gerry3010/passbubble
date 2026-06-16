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

package api

import (
	"fmt"
	"os"
)

type Config struct {
	Host        string
	Port        string
	DatabaseURL string
	RedisURL    string
	JWTSecret   []byte
	AdminEmail  string
	Environment string
}

func LoadConfig() (*Config, error) {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable is required")
	}
	return &Config{
		Host:        getEnv("SERVER_HOST", "127.0.0.1"),
		Port:        getEnv("SERVER_PORT", "8765"),
		DatabaseURL: dbURL,
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:   []byte(jwtSecret),
		AdminEmail:  os.Getenv("ADMIN_EMAIL"),
		Environment: getEnv("ENVIRONMENT", "production"),
	}, nil
}

func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%s", c.Host, c.Port)
}

func (c *Config) IsDevelopment() bool {
	return c.Environment == "development"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
