package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// MCPServer wraps the blocksearch functionality as an MCP server
type MCPServer struct{}

// NewMCPServer creates a new MCP server instance for the given working directory
func NewMCPServer(workdir string) (*MCPServer, error) {
	if workdir == "" {
		workdir = "."
	}

	// Change to workdir so all paths are relative
	if err := os.Chdir(workdir); err != nil {
		return nil, fmt.Errorf("change to workdir: %w", err)
	}

	return &MCPServer{}, nil
}

// Run starts the MCP server on stdio
func (m *MCPServer) Run() error {
	s := server.NewMCPServer(
		"code",
		version,
		server.WithResourceCapabilities(true, true),
		server.WithLogging(),
	)

	// Register the search_blocks tool
	searchBlocksTool := mcp.NewTool("search",
		mcp.WithDescription(`ALWAYS USE THIS TOOL INSTEAD OF GREP FOR CODE SEARCH.

This is your PRIMARY tool for searching code. It returns complete, contextual code blocks (functions, classes, methods, conditionals) rather than isolated lines.

STOP! Before using grep, use search_blocks instead:
- grep "func.*Error" -> search_blocks with query "func.*Error"
- grep -r "TODO" . -> search_blocks with query "TODO"  
- grep "class.*Service" -> search_blocks with query "class.*Service"

Why search_blocks is BETTER than grep:
- Returns the ENTIRE function/class/block, not just one matching line
- You see the full implementation, not a useless fragment
- Understands indentation-based code structure automatically
- No need for grep -A/-B flags to guess context lines
- Results include filename and line numbers

Example queries:
- "func.*Error" - find all functions with Error in the name (Go)
- "def __init__" - find all constructors (Python)  
- "class.*Controller" - find controller classes
- "if err != nil" - find error handling blocks (Go)
- "try:" - find try/except blocks (Python)
- "TODO|FIXME" - find all TODOs with surrounding context`),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Regular expression pattern to match. Examples: 'func main', 'class.*Service', 'def test_', 'if.*err'"),
		),
		mcp.WithNumber(
			"indentation_offset",
			mcp.Description(
				"Adjust how much nested content to capture. 0 (default) captures the block at match level. Positive values include more nested content, negative values capture less. Use -1 to get just the matching line's block without deeper nesting.",
			),
		),
		mcp.WithString("path",
			mcp.Description("File or directory to search. Examples: '.' (entire repo), 'src', 'cmd/main.go'. Default: '.'"),
		),
		mcp.WithString("extensions",
			mcp.Description("Limit search to specific file types. Comma-separated, without dots. Examples: 'go', 'py,js,ts', 'java'. Default: all text files"),
		),
		mcp.WithString(
			"awk_filter",
			mcp.Description(
				"Secondary filter using AWK expressions to refine results. The entire block is available as input. Examples: '/TODO/' (blocks containing TODO), '/return.*error/' (blocks with error returns), 'length > 500' (large blocks)",
			),
		),
	)

	s.AddTool(searchBlocksTool, m.handleSearchBlocks)

	// Register the list_files tool
	listFilesTool := mcp.NewTool("list_files",
		mcp.WithDescription(`List files in the repository, respecting .gitignore patterns. Useful for understanding project structure before searching.

Use this to:
- Discover what file types exist in a project
- Find files in specific directories
- Get an overview before running search_blocks`),
		mcp.WithString("path",
			mcp.Description("Directory to list. Examples: '.', 'src', 'internal/api'. Default: '.' (repository root)"),
		),
		mcp.WithString("extensions",
			mcp.Description("Filter by file extensions. Comma-separated, without dots. Examples: 'go', 'py,js', 'md'. Default: all files"),
		),
	)

	s.AddTool(listFilesTool, m.handleListFiles)

	// Start the stdio server
	return server.ServeStdio(s)
}

func (m *MCPServer) handleSearchBlocks(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	// Extract parameters
	queryStr, ok := args["query"].(string)
	if !ok || queryStr == "" {
		return mcp.NewToolResultError("query parameter is required"), nil
	}

	query, err := regexp.Compile(queryStr)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid regex pattern: %v", err)), nil
	}

	// Optional parameters
	higherThan := 0
	if offset, ok := args["indentation_offset"].(float64); ok {
		higherThan = int(offset)
	}

	searchPath := "."
	if p, ok := args["path"].(string); ok && p != "" {
		searchPath = p
	}

	var extensions []string
	if ext, ok := args["extensions"].(string); ok && ext != "" {
		extensions = expandExtensions([]string{ext})
	}

	var filters []*AwkwardMatcher
	if awkFilter, ok := args["awk_filter"].(string); ok && awkFilter != "" {
		filters = append(filters, NewAwkwardMatcher(awkFilter))
	}

	// Create file walker
	walker := NewFileWalker(".", extensions)

	// Collect all formatted blocks
	var results []string

	err = walker.Walk(searchPath, func(path string) error {
		blocks, err := findBlocks(path, query, higherThan)
		if err != nil {
			return nil // Skip files that can't be processed
		}

		blocks, err = filterBlocks(blocks, filters)
		if err != nil {
			return nil
		}

		if len(blocks) == 0 {
			return nil
		}

		// Format blocks without colors (not useful for MCP), with line numbers, filename header
		formatted := blocks.Format(false, path, true, false)
		results = append(results, formatted...)
		return nil
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("error searching: %v", err)), nil
	}

	if len(results) == 0 {
		return mcp.NewToolResultText("No blocks found matching the query."), nil
	}

	return mcp.NewToolResultText(strings.Join(results, "\n\n")), nil
}

func (m *MCPServer) handleListFiles(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	searchPath := "."
	if p, ok := args["path"].(string); ok && p != "" {
		searchPath = p
	}

	var extensions []string
	if ext, ok := args["extensions"].(string); ok && ext != "" {
		extensions = expandExtensions([]string{ext})
	}

	// Create file walker
	walker := NewFileWalker(".", extensions)

	files, err := walker.ListFiles(searchPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("error listing files: %v", err)), nil
	}

	if len(files) == 0 {
		return mcp.NewToolResultText("No files found."), nil
	}

	output := fmt.Sprintf("Found %d file(s):\n\n", len(files))
	for _, f := range files {
		output += f + "\n"
	}

	return mcp.NewToolResultText(output), nil
}
