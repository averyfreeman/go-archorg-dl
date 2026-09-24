package archorgdl

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const maxVerbosity = 3

type processResult struct {
	Executable string
	Args       []string
	ExitCode   int
	Stdout     string
	Stderr     string
}

func persistProcessResult(workdir, name string, result processResult) error {
	if err := os.MkdirAll(workdir, 0o755); err != nil {
		return err
	}
	files := map[string]string{
		name + ".command": formatCommand(result.Executable, result.Args),
		name + ".status":  strconv.Itoa(result.ExitCode) + "\n",
		name + ".stdout":  result.Stdout,
		name + ".stderr":  result.Stderr,
	}
	for filename, content := range files {
		if err := os.WriteFile(filepath.Join(workdir, filename), []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func formatCommand(executable string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	if executable != "" {
		parts = append(parts, strconv.Quote(executable))
	}
	for _, arg := range args {
		parts = append(parts, strconv.Quote(arg))
	}
	return strings.Join(parts, " ") + "\n"
}

func logInfo(verbosity int, message string, args ...any) {
	if verbosity >= 1 {
		slog.Default().Info(message, args...)
	}
}

func logDebug(verbosity int, message string, args ...any) {
	if verbosity >= 2 {
		slog.Default().Debug(message, args...)
	}
}

func logRawProcessResult(verbosity int, name string, result processResult) {
	if verbosity < maxVerbosity {
		return
	}
	if strings.TrimSpace(result.Stdout) != "" {
		slog.Default().Debug(name+" stdout", "output", result.Stdout)
	}
	if strings.TrimSpace(result.Stderr) != "" {
		slog.Default().Debug(name+" stderr", "output", result.Stderr)
	}
}

func processFailure(result processResult, runErr error) error {
	detail := strings.TrimSpace(result.Stderr)
	if detail == "" {
		detail = strings.TrimSpace(result.Stdout)
	}
	if detail == "" && runErr != nil {
		detail = runErr.Error()
	}
	if detail == "" {
		detail = "process failed"
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("exit code %d: %s", result.ExitCode, detail)
	}
	return fmt.Errorf("%s", detail)
}
