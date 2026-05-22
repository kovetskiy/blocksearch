package main

import (
	"strings"
	"unicode"
)

const (
	hashlineFNVOffset = uint32(0x811c9dc5)
	hashlineFNVPrime  = uint32(0x01000193)
)

func hashlineHashes(contents []byte) []string {
	lines := hashlineLines(contents)
	hashes := make([]string, len(lines))
	for i := 0; i < len(lines); i++ {
		hashes[i] = hashlineHashForLine(lines, i)
	}
	return hashes
}

func hashlineLines(contents []byte) []string {
	return strings.Split(hashlineNormalizeFile(contents), "\n")
}

func hashlineNormalizeFile(contents []byte) string {
	if len(contents) >= 3 && contents[0] == 0xef && contents[1] == 0xbb && contents[2] == 0xbf {
		contents = contents[3:]
	}

	text := string(contents)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text
}

func hashlineHashForLine(lines []string, index int) string {
	prev := ""
	if index > 0 {
		prev = hashlineNormalizeLine(lines[index-1])
	}

	curr := hashlineNormalizeLine(lines[index])

	next := ""
	if index < len(lines)-1 {
		next = hashlineNormalizeLine(lines[index+1])
	}

	hash := hashlineFNVOffset
	hash = hashlineFoldString(hash, prev)
	hash = hashlineFoldByte(hash, 0)
	hash = hashlineFoldString(hash, curr)
	hash = hashlineFoldByte(hash, 0)
	hash = hashlineFoldString(hash, next)

	return hashlineHexByte(byte(hash))
}

func hashlineNormalizeLine(line string) string {
	line = strings.ReplaceAll(line, "\r", "")
	return strings.TrimRightFunc(line, unicode.IsSpace)
}

func hashlineFoldString(hash uint32, text string) uint32 {
	for _, ch := range text {
		hash = hashlineFoldRune(hash, ch)
	}
	return hash
}

func hashlineFoldRune(hash uint32, ch rune) uint32 {
	return (hash ^ uint32(ch)) * hashlineFNVPrime
}

func hashlineFoldByte(hash uint32, value byte) uint32 {
	return (hash ^ uint32(value)) * hashlineFNVPrime
}

func hashlineHexByte(value byte) string {
	const digits = "0123456789ABCDEF"
	return string([]byte{digits[value>>4], digits[value&0x0f]})
}
