BLOCKSEARCH(1)                    User Commands                   BLOCKSEARCH(1)

NAME
       blocksearch - search and extract indented code blocks with syntax highlighting

SYNOPSIS
       blocksearch [OPTIONS] PATTERN [FILE...]
       blocksearch -h | --help
       blocksearch --version

DESCRIPTION
       blocksearch is a specialized text search tool designed to find and extract
       logically related blocks of code or text based on indentation levels. Unlike
       traditional line-oriented search tools like grep, blocksearch understands
       the hierarchical structure of indented content and can capture entire code
       blocks, functions, or nested structures that match a given pattern.

       The tool excels at searching through source code, configuration files, and
       any structured text that uses indentation to denote logical relationships.
       When a matching line is found, blocksearch intelligently extends the match
       to include all related lines at higher indentation levels, effectively
       capturing complete code blocks, function definitions, or nested structures.

       blocksearch provides terminal-based syntax highlighting using the Chroma
       library, making the output highly readable when viewing code blocks in
       the terminal. The syntax highlighting is automatically detected based on
       file extensions and can be disabled when needed.

BLOCK EXTRACTION ALGORITHM
       When blocksearch finds a line matching the search pattern, it applies the
       following algorithm to determine the complete block:

       1. Record the indentation level of the matching line
       2. Include all subsequent lines that are:
          - Empty lines (considered part of the block structure)
          - Lines with indentation greater than the base level plus the offset
            specified by the -i option
       3. Stop when encountering a line at or below the base indentation level
       4. Optionally include one additional line at the base level to provide
          context for the block's termination

       This approach ensures that complete logical units are extracted, such as:
       - Entire function definitions with their bodies
       - Complete class definitions with all methods
       - Full conditional blocks with all nested statements
       - Configuration sections with all nested parameters

OPTIONS
       -i N   Show lines with indentation higher than the matching line's level
              plus N. This value can be negative to capture blocks at lower
              indentation levels. Default behavior includes all higher-indented
              content.

       -t, --file
              Prefix each line with the filename. Useful when searching multiple
              files or when output will be processed by other tools.

       -l, --no-line
              Suppress line numbers in the output. By default, each line is
              prefixed with its line number in the source file.

       -c, --no-colors
              Disable syntax highlighting. Output will be plain text without
              ANSI color codes. Useful for redirecting output to files or
              processing with other tools.

       -j, --json
              Output results in JSON format. Each block is represented as a
              JSON object containing filename, line range, and text content.
              This format is suitable for programmatic processing.

       -S, --stream COMMAND
              Stream each block to the specified command as JSON input. The
              command is executed once for each matching block, receiving the
              block data via stdin. This enables real-time processing of
              search results.

       -a, --awk CONDITION
              Filter blocks using AWK expressions. Only blocks where the AWK
              condition evaluates to true will be included in the output. The
              entire block text is available as $0 in the AWK expression.
              Multiple -a options can be specified; blocks matching any
              condition will be included.

       -e, --exit-code CODE
              Exit with the specified code when blocks are found. Default is 0.
              Useful in scripts where finding matches should trigger specific
              behavior or error handling.

       --message MESSAGE
              Display the specified message when blocks are found. This can be
              used to provide context or warnings when matches are discovered.

       -x, --extension EXT
              Limit search to files with the specified extensions. Multiple
              extensions can be specified with multiple -x options or by
              separating extensions with commas. Extensions should be specified
              without the leading dot (e.g., "go", "py", "js").

       -v     Enable verbose output for debugging and detailed operation
              information.

       --version
              Display version information and exit.

       -h, --help
              Display usage information and exit.

ARGUMENTS
       PATTERN
              Regular expression pattern to search for. The pattern is applied
              to each line of input files. When a match is found, the entire
              logical block containing that line is extracted according to the
              indentation rules.

       FILE...
              Files or directories to search. If directories are specified,
              they are recursively traversed. If no files are specified and
              stdin is a terminal, the current directory is searched. If stdin
              is not a terminal, input is read from stdin.

SYNTAX HIGHLIGHTING
       blocksearch automatically detects file types based on extensions and
       applies appropriate syntax highlighting using the Chroma library. The
       highlighting uses a vim-compatible color scheme optimized for terminal
       display.

       Supported languages include most common programming languages, markup
       formats, and configuration file types. When file type cannot be
       determined, a fallback lexer provides basic highlighting.

       Syntax highlighting can be disabled with the -c/--no-colors option,
       which is automatically applied when output is redirected to a file
       or pipe.

GITIGNORE INTEGRATION
       blocksearch respects .gitignore files in the search directory. Files
       and directories matching .gitignore patterns are automatically excluded
       from the search. This prevents searching through build artifacts,
       dependency directories, and other files typically excluded from
       version control.

       The .git directory is always excluded from recursive searches.

OUTPUT FORMATS
       Default Format:
              Each matching block is displayed with syntax highlighting (if
              enabled), line numbers, and clear separation between blocks.
              Multiple blocks are separated by blank lines.

       JSON Format (-j):
              Each block is output as a JSON object with fields:
              - filename: source file path
              - line_start: first line number of the block
              - line_end: last line number of the block
              - text: complete block content

       Streaming Format (-S):
              Identical to JSON format but each block is immediately passed
              to the specified command for processing.

EXAMPLES
       Search for function definitions in Go files:
              blocksearch "func.*{" *.go

       Find all error handling blocks with context:
              blocksearch -i 1 "if.*err" .

       Extract complete class definitions from Python files:
              blocksearch "^class " -x py

       Search with syntax highlighting disabled:
              blocksearch -c "TODO" src/

       Output results in JSON format:
              blocksearch -j "struct.*{" *.go

       Filter blocks containing specific patterns:
              blocksearch -a '/panic/' "func.*{" *.go

       Search specific file types recursively:
              blocksearch -x go,js,py "function\|func\|def" .

       Stream results to a processing script:
              blocksearch -S ./process-block.sh "FIXME" .

EXIT STATUS
       0      No blocks found or successful completion
       N      Blocks were found and -e/--exit-code N was specified
       1      Error occurred during execution

ENVIRONMENT
       The tool respects standard terminal environment variables for color
       support detection and terminal capabilities.

FILES
       .gitignore
              Git ignore patterns file. When present, matching files and
              directories are excluded from search.

AUTHOR
       This implementation uses the Go programming language and integrates
       several open-source libraries for regular expressions, syntax
       highlighting, and AWK expression evaluation.

LICENSE
       MIT License. This software is provided as-is without warranty.

SEE ALSO
       grep(1), awk(1), find(1), git-grep(1)

       For more information about regular expression syntax, see the Go
       regexp package documentation.

blocksearch v1.4.0                 June 2025                    BLOCKSEARCH(1)
