package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds all user-tunable settings with defaults applied.
type Config struct {
	Monitoring MonitoringConfig `toml:"monitoring"`
	Grouping   GroupingConfig   `toml:"grouping"`
	Autostart  AutostartConfig  `toml:"autostart"`
}

type MonitoringConfig struct {
	PollIntervalSecs int `toml:"poll_interval_secs"`
	MinSessionSecs   int `toml:"min_session_secs"`
	IdleTimeoutMins  int `toml:"idle_timeout_mins"`
	CheckpointSecs   int `toml:"checkpoint_secs"`
}

type GroupingConfig struct {
	BrowserAdjacencyMins int `toml:"browser_adjacency_mins"`
}

type AutostartConfig struct {
	Enabled bool `toml:"enabled"`
}

func defaults() Config {
	return Config{
		Monitoring: MonitoringConfig{
			PollIntervalSecs: 5,
			MinSessionSecs:   30,
			IdleTimeoutMins:  10,
			CheckpointSecs:   60,
		},
		Grouping: GroupingConfig{
			BrowserAdjacencyMins: 15,
		},
		Autostart: AutostartConfig{
			Enabled: true,
		},
	}
}

// DataDir returns the directory used for the database and config file.
func DataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".activitytracker"), nil
}

// ConfigPath returns the full path to the TOML config file.
func ConfigPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// Load reads the config file and applies defaults for any missing fields.
// If the file does not exist, returns all defaults without error.
func Load() (Config, error) {
	cfg := defaults()
	path, err := ConfigPath()
	if err != nil {
		return cfg, err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return defaults(), err
	}
	clamp(&cfg)
	return cfg, nil
}

func clamp(cfg *Config) {
	if cfg.Monitoring.PollIntervalSecs < 1 {
		cfg.Monitoring.PollIntervalSecs = 1
	}
	if cfg.Monitoring.IdleTimeoutMins < 1 {
		cfg.Monitoring.IdleTimeoutMins = 1
	}
}
