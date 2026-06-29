package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// HelpMode selects which help text to print.
type HelpMode int

const (
	HelpNone HelpMode = iota
	HelpBrief
	HelpAll
)

// PreprocessArgs extracts --env-file, --help, and --help-all before flag parsing.
// Remaining args are passed to Parse. explicit is true when --env-file was passed.
func PreprocessArgs(args []string) (remaining []string, envFile string, explicit bool, help HelpMode, err error) {
	envFile = ".env"

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--help" || arg == "-h":
			return nil, "", false, HelpBrief, nil
		case arg == "--help-all":
			return nil, "", false, HelpAll, nil
		case arg == "--env-file":
			if i+1 >= len(args) {
				return nil, "", false, HelpNone, fmt.Errorf("--env-file requires a path")
			}
			envFile = args[i+1]
			explicit = true
			i++
		case strings.HasPrefix(arg, "--env-file="):
			envFile = strings.TrimPrefix(arg, "--env-file=")
			explicit = true
		default:
			remaining = append(remaining, arg)
		}
	}

	return remaining, envFile, explicit, HelpNone, nil
}

// LoadEnvFile loads KEY=VALUE pairs into the process environment. Existing shell
// variables are not overwritten (godotenv.Load only sets unset keys). When path is
// the default ".env" and the file is missing, loading is skipped silently; an
// explicitly-requested missing file is an error.
func LoadEnvFile(path string, explicit bool) error {
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if explicit {
			return fmt.Errorf("env file not found: %s", path)
		}
		return nil
	}
	return godotenv.Load(path)
}
