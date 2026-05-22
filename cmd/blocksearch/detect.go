package main

import (
	"bytes"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/bash"
	treec "github.com/smacker/go-tree-sitter/c"
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/csharp"
	"github.com/smacker/go-tree-sitter/css"
	"github.com/smacker/go-tree-sitter/cue"
	"github.com/smacker/go-tree-sitter/dockerfile"
	"github.com/smacker/go-tree-sitter/elixir"
	"github.com/smacker/go-tree-sitter/elm"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/groovy"
	"github.com/smacker/go-tree-sitter/hcl"
	"github.com/smacker/go-tree-sitter/html"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/kotlin"
	"github.com/smacker/go-tree-sitter/lua"
	treemd "github.com/smacker/go-tree-sitter/markdown/tree-sitter-markdown"
	"github.com/smacker/go-tree-sitter/ocaml"
	"github.com/smacker/go-tree-sitter/php"
	"github.com/smacker/go-tree-sitter/protobuf"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/scala"
	"github.com/smacker/go-tree-sitter/sql"
	"github.com/smacker/go-tree-sitter/svelte"
	"github.com/smacker/go-tree-sitter/swift"
	"github.com/smacker/go-tree-sitter/toml"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
	"github.com/smacker/go-tree-sitter/yaml"
)

type languageFactory func() *sitter.Language

var extensionLanguages = map[string]languageFactory{
	".bash":       bash.GetLanguage,
	".bats":       bash.GetLanguage,
	".c":          treec.GetLanguage,
	".cc":         cpp.GetLanguage,
	".cjs":        javascript.GetLanguage,
	".cpp":        cpp.GetLanguage,
	".cs":         csharp.GetLanguage,
	".css":        css.GetLanguage,
	".cue":        cue.GetLanguage,
	".cxx":        cpp.GetLanguage,
	".c++":        cpp.GetLanguage,
	".dockerfile": dockerfile.GetLanguage,
	".elm":        elm.GetLanguage,
	".ex":         elixir.GetLanguage,
	".exs":        elixir.GetLanguage,
	".go":         golang.GetLanguage,
	".gradle":     groovy.GetLanguage,
	".groovy":     groovy.GetLanguage,
	".h":          treec.GetLanguage,
	".hcl":        hcl.GetLanguage,
	".hh":         cpp.GetLanguage,
	".hpp":        cpp.GetLanguage,
	".hxx":        cpp.GetLanguage,
	".h++":        cpp.GetLanguage,
	".htm":        html.GetLanguage,
	".html":       html.GetLanguage,
	".java":       java.GetLanguage,
	".jenkins":    groovy.GetLanguage,
	".js":         javascript.GetLanguage,
	".json":       javascript.GetLanguage,
	".jsx":        javascript.GetLanguage,
	".kt":         kotlin.GetLanguage,
	".kts":        kotlin.GetLanguage,
	".lua":        lua.GetLanguage,
	".markdown":   treemd.GetLanguage,
	".md":         treemd.GetLanguage,
	".mjs":        javascript.GetLanguage,
	".ml":         ocaml.GetLanguage,
	".mli":        ocaml.GetLanguage,
	".php":        php.GetLanguage,
	".proto":      protobuf.GetLanguage,
	".py":         python.GetLanguage,
	".pyw":        python.GetLanguage,
	".rb":         ruby.GetLanguage,
	".rs":         rust.GetLanguage,
	".scala":      scala.GetLanguage,
	".sc":         scala.GetLanguage,
	".sh":         bash.GetLanguage,
	".ksh":        bash.GetLanguage,
	".zsh":        bash.GetLanguage,
	".sql":        sql.GetLanguage,
	".svelte":     svelte.GetLanguage,
	".swift":      swift.GetLanguage,
	".tf":         hcl.GetLanguage,
	".tfvars":     hcl.GetLanguage,
	".toml":       toml.GetLanguage,
	".ts":         typescript.GetLanguage,
	".tsx":        tsx.GetLanguage,
	".yaml":       yaml.GetLanguage,
	".yml":        yaml.GetLanguage,
}

// filenameLanguages handles extensionless files whose name alone implies
// the language, e.g. a Dockerfile or Containerfile.
var filenameLanguages = map[string]languageFactory{
	"dockerfile":    dockerfile.GetLanguage,
	"containerfile": dockerfile.GetLanguage,
	"jenkinsfile":   groovy.GetLanguage,
}

func detectLanguage(path string, contents []byte) *sitter.Language {
	if language := languageForExtension(path); language != nil {
		return language
	}

	return languageForShebang(contents)
}

func languageForExtension(path string) *sitter.Language {
	if factory := extensionLanguages[strings.ToLower(filepath.Ext(path))]; factory != nil {
		return factory()
	}

	if factory := filenameLanguages[strings.ToLower(filepath.Base(path))]; factory != nil {
		return factory()
	}

	return nil
}

func languageForShebang(contents []byte) *sitter.Language {
	contents = bytes.TrimPrefix(contents, []byte("\xef\xbb\xbf"))
	line, _, _ := bytes.Cut(contents, []byte("\n"))
	if !bytes.HasPrefix(line, []byte("#!")) {
		return nil
	}

	shebang := strings.ToLower(string(line))
	switch {
	case strings.Contains(shebang, "python"):
		return python.GetLanguage()
	case strings.Contains(shebang, "node"),
		strings.Contains(shebang, "deno"),
		strings.Contains(shebang, "bun"),
		strings.Contains(shebang, "javascript"):
		return javascript.GetLanguage()
	case strings.Contains(shebang, "ruby"):
		return ruby.GetLanguage()
	case strings.Contains(shebang, "bash"),
		strings.Contains(shebang, "zsh"),
		strings.Contains(shebang, "ksh"),
		strings.Contains(shebang, " sh"),
		strings.Contains(shebang, "/sh"):
		return bash.GetLanguage()
	default:
		return nil
	}
}
