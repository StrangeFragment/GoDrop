package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	appDirectoryName  = "GoArchivist"
	configFileName    = "config.json"
	databaseFileName  = "goarchivist.db"
	logsDirectoryName = "logs"
)

type Paths struct {
	ConfigDir  string
	ConfigFile string
	Database   string
	LogsDir    string
}

type Config struct {
	Web     WebConfig     `json:"web"`
	IMAP    IMAPConfig    `json:"imap"`
	Archive ArchiveConfig `json:"archive"`
}

type WebConfig struct {
	Address string `json:"address"`
}

type IMAPConfig struct {
	Server   string `json:"server"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Mailbox  string `json:"mailbox"`
}

type ArchiveConfig struct {
	Directory string `json:"directory"`
}

func Default() Config {
	return Config{
		Web: WebConfig{
			Address: "127.0.0.1:8080",
		},
		IMAP: IMAPConfig{
			Port:    993,
			Mailbox: "INBOX",
		},
		Archive: ArchiveConfig{
			Directory: "",
		},
	}
}

func UserPaths() (Paths, error) {
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, fmt.Errorf(
			"не удалось определить папку настроек пользователя: %w",
			err,
		)
	}

	configDir := filepath.Join(configRoot, appDirectoryName)

	paths := Paths{
		ConfigDir:  configDir,
		ConfigFile: filepath.Join(configDir, configFileName),
		Database:   filepath.Join(configDir, databaseFileName),
		LogsDir:    filepath.Join(configDir, logsDirectoryName),
	}

	for _, dir := range []string{paths.ConfigDir, paths.LogsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Paths{}, fmt.Errorf("не удалось создать папку %q: %w", dir, err)
		}
	}

	return paths, nil
}

func Load() (Config, Paths, error) {
	paths, err := UserPaths()
	if err != nil {
		return Config{}, Paths{}, err
	}

	data, err := os.ReadFile(paths.ConfigFile)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Config{}, Paths{}, fmt.Errorf("ошибка чтения config.json: %w", err)
		}

		cfg := Default()
		if err := Save(paths.ConfigFile, cfg); err != nil {
			return Config{}, Paths{}, fmt.Errorf("не удалось создать config.json: %w", err)
		}

		return cfg, paths, nil
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, Paths{}, fmt.Errorf("ошибка разбора config.json: %w", err)
	}

	applyDefaults(&cfg)
	return cfg, paths, nil
}

func Save(path string, cfg Config) error {
	if path == "" {
		return errors.New("путь к файлу настроек не задан")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("не удалось создать папку настроек: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("не удалось подготовить config.json: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("не удалось сохранить config.json: %w", err)
	}

	return nil
}

func applyDefaults(cfg *Config) {
	defaults := Default()

	if cfg.Web.Address == "" {
		cfg.Web.Address = defaults.Web.Address
	}
	if cfg.IMAP.Port == 0 {
		cfg.IMAP.Port = defaults.IMAP.Port
	}
	if cfg.IMAP.Mailbox == "" {
		cfg.IMAP.Mailbox = defaults.IMAP.Mailbox
	}

	// Archive.Directory намеренно не заполняется автоматически:
	// папку хранилища пользователь выбирает отдельно.
}
