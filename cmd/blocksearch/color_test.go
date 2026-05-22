package main

import (
	"testing"
)

// --color never/always/auto resolve to the color policy. "auto" follows
// stdout, so under a non-terminal stdout (as in `go test`) it is off.
func TestColorPolicyResolvesNeverAlwaysAuto(t *testing.T) {
	never := buildSearchForTest(t, "--color", "never", "query", "unused")
	if never.output.UseColors {
		t.Fatalf("never: UseColors = true, want false")
	}

	always := buildSearchForTest(t, "--color", "always", "query", "unused")
	if !always.output.UseColors {
		t.Fatalf("always: UseColors = false, want true")
	}

	auto := buildSearchForTest(t, "--color", "auto", "query", "unused")
	if auto.output.UseColors {
		t.Fatalf("auto: UseColors = true, want false under non-terminal stdout")
	}
}

// The default is auto, so omitting --color matches an explicit --color=auto.
func TestColorPolicyDefaultsToAuto(t *testing.T) {
	def := buildSearchForTest(t, "query", "unused")
	auto := buildSearchForTest(t, "--color", "auto", "query", "unused")
	if def.output.UseColors != auto.output.UseColors {
		t.Fatalf("default UseColors = %v, auto = %v; default must equal auto",
			def.output.UseColors, auto.output.UseColors)
	}
}

// An invalid --color value must fail at search construction, not silently
// fall back to a default.
func TestColorPolicyRejectsInvalidValue(t *testing.T) {
	args, err := parseArgumentsArgs([]string{"--color", "yes", "query", "unused"})
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if _, err := buildSearch(args); err == nil {
		t.Fatalf("buildSearch: err = nil, want non-nil for invalid --color")
	}
}

// --color accepts the attached form (--color=never) the same as the
// space-separated form, mirroring grep.
func TestColorPolicyAcceptsAttachedValue(t *testing.T) {
	search := buildSearchForTest(t, "--color=never", "query", "unused")
	if search.output.UseColors {
		t.Fatalf("attached --color=never: UseColors = true, want false")
	}
}
