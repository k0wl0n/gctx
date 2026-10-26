package tui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

// metacharacters that are rejected outside quotes.
const metachar = "|;&><`"

// windowsBatchExts are extensions whose batch wrappers expand special chars.
var windowsBatchExts = []string{".cmd", ".bat"}

// windowsDangerChars are argument characters rejected when the CLI is a batch wrapper.
const windowsDangerChars = "&|<>^%!\"\n"

// tokenize splits run-prompt input into an argument vector for the cli
// executable bin. A leading token naming cli is dropped. On error the message is suitable
// for inline display; the caller should not execute anything.
func tokenize(input, cli, bin, goos string) ([]string, error) {
	tokens, err := splitTokens(input)
	if err != nil {
		return nil, err
	}
	if len(tokens) == 0 {
		return nil, nil
	}
	if tokens[0] == cli {
		tokens = tokens[1:]
	}
	if len(tokens) == 0 {
		return nil, nil
	}
	// On Windows a .cmd/.bat wrapper expands these characters.
	if goos == "windows" && isBatchBin(bin) {
		for _, arg := range tokens {
			if strings.ContainsAny(arg, windowsDangerChars) {
				return nil, fmt.Errorf("argument contains characters that are unsafe with %s%s; press s for an isolated shell", cli, strings.ToLower(filepath.Ext(bin)))
			}
		}
	}
	return tokens, nil
}

func isBatchBin(bin string) bool {
	ext := strings.ToLower(filepath.Ext(bin))
	for _, e := range windowsBatchExts {
		if ext == e {
			return true
		}
	}
	return false
}

// splitTokens parses input into tokens applying the quoting rules.
func splitTokens(input string) ([]string, error) {
	var tokens []string
	var cur strings.Builder
	inToken := false
	r := []rune(input)
	i := 0
	for i < len(r) {
		ch := r[i]
		switch {
		case ch == '\'':
			// Single-quoted: everything literal until closing '.
			i++
			inToken = true
			for i < len(r) && r[i] != '\'' {
				cur.WriteRune(r[i])
				i++
			}
			if i >= len(r) {
				return nil, errors.New("unterminated single quote")
			}
			i++ // consume closing '
		case ch == '"':
			// Double-quoted: backslash escapes " and \; everything else literal.
			i++
			inToken = true
			for i < len(r) && r[i] != '"' {
				if r[i] == '\\' && i+1 < len(r) && (r[i+1] == '"' || r[i+1] == '\\') {
					i++
					cur.WriteRune(r[i])
				} else {
					cur.WriteRune(r[i])
				}
				i++
			}
			if i >= len(r) {
				return nil, errors.New("unterminated double quote")
			}
			i++ // consume closing "
		case ch == '\\':
			// Outside quotes: escapes whitespace, quotes, and backslash.
			// Any other backslash is literal (both the backslash and the next char).
			inToken = true
			if i+1 < len(r) {
				next := r[i+1]
				if unicode.IsSpace(next) || next == '\'' || next == '"' || next == '\\' {
					cur.WriteRune(next)
					i += 2
				} else {
					cur.WriteRune(ch)
					i++
				}
			} else {
				cur.WriteRune(ch)
				i++
			}
		case strings.ContainsRune(metachar, ch) || (ch == '$' && i+1 < len(r) && r[i+1] == '(') || ch == '`':
			// Unquoted metacharacter (including $( sequence).
			return nil, errors.New("Shell operators are not supported here; press s for an isolated shell")
		case unicode.IsSpace(ch):
			if inToken {
				tokens = append(tokens, cur.String())
				cur.Reset()
				inToken = false
			}
			i++
		default:
			cur.WriteRune(ch)
			inToken = true
			i++
		}
	}
	if inToken {
		tokens = append(tokens, cur.String())
	}
	return tokens, nil
}
