package main

import (
	"reflect"
	"testing"
)

func TestNormalizeVerbosityArgs(t *testing.T) {
	got := normalizeVerbosityArgs([]string{"-v", "-vv", "-vvv", "-vvvv", "https://archive.org/details/example"})
	want := []string{"-v", "-v=2", "-v=3", "-v=3", "https://archive.org/details/example"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeVerbosityArgs() = %#v, want %#v", got, want)
	}
}

func TestParseOptionsDefaultsAreDiagnosticFriendly(t *testing.T) {
	options, err := parseOptions([]string{"-vvv", "https://archive.org/details/example"})
	if err != nil {
		t.Fatal(err)
	}
	if options.NoOverwrite || options.CleanupWork || options.ReuseWork {
		t.Fatalf("defaults = %#v", options)
	}
	if options.Verbosity != 3 {
		t.Fatalf("verbosity = %d, want 3", options.Verbosity)
	}
}

func TestParseOptionsSupportsExplicitOptOuts(t *testing.T) {
	options, err := parseOptions([]string{
		"--no-overwrite",
		"--cleanup-work",
		"--reuse-work",
		"-v",
		"https://archive.org/details/example",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !options.NoOverwrite || !options.CleanupWork || !options.ReuseWork || options.Verbosity != 1 {
		t.Fatalf("options = %#v", options)
	}
}
