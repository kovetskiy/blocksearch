# Testdata Source

A grab-bag of source files exercising supported grammars and parser edge cases.

## Why this exists

The fixture files and their header comments are the source of truth for the corpus.

- **main.py** covers canonical 4-space Python.
- **utils.py** covers legal 2-space Python.
- **Makefile** exercises indentation-based fallback with mandatory tabs.
- **config.yaml** covers indentation-sensitive YAML.
- **handler.c** and **server.rs** cover nested brace-based languages.
- The remaining files provide focused grammar and regression fixtures.

Use the directory listing when adding tests rather than maintaining a duplicate
exhaustive inventory here.

## Note

This file is prose, not code. It exists to test that blocksearch correctly
handles Markdown input.
