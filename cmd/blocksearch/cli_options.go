package main

import "fmt"

func validateSearchArguments(args Arguments) error {
	switch args.ValueOverlap {
	case "", "all", "outermost", "innermost":
	default:
		return fmt.Errorf("invalid --overlap %q: want all, outermost, or innermost", args.ValueOverlap)
	}
	switch args.ValueDiagnostics {
	case "", "text", "json":
	default:
		return fmt.Errorf("invalid --diagnostics %q: want text or json", args.ValueDiagnostics)
	}
	if args.ValueExitCode < 0 || args.ValueExitCode > 255 {
		return fmt.Errorf("--exit-code must be between 0 and 255")
	}
	stream := args.ValueStreamCommand != ""
	persistent := args.ValuePersistentStreamCommand != ""
	if stream && persistent {
		return fmt.Errorf("--stream and --stream-persistent are mutually exclusive")
	}
	if args.FlagFilesOnly && (stream || persistent || args.FlagMatches) {
		return fmt.Errorf("--files cannot be combined with streaming or --matches")
	}
	if args.FlagNull && (!args.FlagFilesOnly || args.FlagJSON || args.FlagMatches) {
		return fmt.Errorf("--null requires --files and cannot be combined with JSON")
	}
	for _, limit := range []struct {
		name  string
		value int64
	}{
		{"--max-file-bytes", args.ValueMaxFileBytes},
		{"--max-block-lines", int64(args.ValueMaxBlockLines)},
		{"--max-block-bytes", args.ValueMaxBlockBytes},
		{"--max-matches", int64(args.ValueMaxMatches)},
		{"--max-blocks", int64(args.ValueMaxBlocks)},
		{"--max-output-bytes", args.ValueMaxOutputBytes},
	} {
		if limit.value < 0 {
			return fmt.Errorf("%s must be nonnegative", limit.name)
		}
	}
	return nil
}

func limitsFromArgs(args Arguments) SearchLimits {
	return SearchLimits{
		FileBytes:   args.ValueMaxFileBytes,
		BlockLines:  args.ValueMaxBlockLines,
		BlockBytes:  args.ValueMaxBlockBytes,
		Matches:     args.ValueMaxMatches,
		Blocks:      args.ValueMaxBlocks,
		OutputBytes: args.ValueMaxOutputBytes,
	}
}
