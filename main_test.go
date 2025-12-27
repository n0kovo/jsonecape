package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestJsonEscape(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		asciiOnly bool
		htmlSafe  bool
		expected  string
	}{
		{
			name:     "simple string",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "quotes",
			input:    `say "hello"`,
			expected: `say \"hello\"`,
		},
		{
			name:     "backslash",
			input:    `path\to\file`,
			expected: `path\\to\\file`,
		},
		{
			name:     "newline",
			input:    "line1\nline2",
			expected: `line1\nline2`,
		},
		{
			name:     "tab",
			input:    "col1\tcol2",
			expected: `col1\tcol2`,
		},
		{
			name:     "carriage return",
			input:    "line1\r\nline2",
			expected: `line1\r\nline2`,
		},
		{
			name:     "all special chars",
			input:    "\b\f\n\r\t\"\\",
			expected: `\b\f\n\r\t\"\\`,
		},
		{
			name:     "control characters",
			input:    "hello\x00\x1fworld",
			expected: `hello\u0000\u001fworld`,
		},
		{
			name:     "unicode preserved",
			input:    "日本語",
			expected: "日本語",
		},
		{
			name:      "unicode escaped with ascii mode",
			input:     "日本語",
			asciiOnly: true,
			expected:  `\u65e5\u672c\u8a9e`,
		},
		{
			name:      "emoji with ascii mode",
			input:     "Hello 👋",
			asciiOnly: true,
			expected:  `Hello \ud83d\udc4b`,
		},
		{
			name:     "html characters preserved by default",
			input:    "<script>&</script>",
			expected: "<script>&</script>",
		},
		{
			name:     "html characters escaped in html-safe mode",
			input:    "<script>&</script>",
			htmlSafe: true,
			expected: `\u003cscript\u003e\u0026\u003c/script\u003e`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := jsonEscape(tt.input, tt.asciiOnly, tt.htmlSafe)
			if result != tt.expected {
				t.Errorf("jsonEscape(%q, %v, %v) = %q, want %q",
					tt.input, tt.asciiOnly, tt.htmlSafe, result, tt.expected)
			}
		})
	}
}

func TestJsonUnescape(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "simple string",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "escaped quotes",
			input:    `say \"hello\"`,
			expected: `say "hello"`,
		},
		{
			name:     "escaped backslash",
			input:    `path\\to\\file`,
			expected: `path\to\file`,
		},
		{
			name:     "escaped newline",
			input:    `line1\nline2`,
			expected: "line1\nline2",
		},
		{
			name:     "escaped tab",
			input:    `col1\tcol2`,
			expected: "col1\tcol2",
		},
		{
			name:     "all escapes",
			input:    `\b\f\n\r\t\"\\\/`,
			expected: "\b\f\n\r\t\"\\/",
		},
		{
			name:     "unicode escape",
			input:    `\u0048\u0065\u006c\u006c\u006f`,
			expected: "Hello",
		},
		{
			name:     "unicode japanese",
			input:    `\u65e5\u672c\u8a9e`,
			expected: "日本語",
		},
		{
			name:     "surrogate pair",
			input:    `\ud83d\udc4b`,
			expected: "👋",
		},
		{
			name:     "mixed content",
			input:    `Hello\nWorld \u0021`,
			expected: "Hello\nWorld !",
		},
		{
			name:    "incomplete escape",
			input:   `hello\`,
			wantErr: true,
		},
		{
			name:    "invalid escape char",
			input:   `hello\x`,
			wantErr: true,
		},
		{
			name:    "incomplete unicode",
			input:   `hello\u00`,
			wantErr: true,
		},
		{
			name:    "invalid unicode hex",
			input:   `hello\uXXXX`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := jsonUnescape(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("jsonUnescape(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("jsonUnescape(%q) unexpected error: %v", tt.input, err)
				return
			}
			if result != tt.expected {
				t.Errorf("jsonUnescape(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	tests := []string{
		"hello world",
		"line1\nline2\nline3",
		`quotes "and" 'stuff'`,
		"日本語テスト",
		"emoji: 👋🌍🎉",
		"\x00\x01\x02\x1f",
		"mixed: hello\tworld\n日本語",
	}

	for _, input := range tests {
		t.Run(input[:min(20, len(input))], func(t *testing.T) {
			escaped := jsonEscape(input, false, false)
			unescaped, err := jsonUnescape(escaped)
			if err != nil {
				t.Errorf("round trip failed: escape(%q) = %q, unescape error: %v",
					input, escaped, err)
				return
			}
			if unescaped != input {
				t.Errorf("round trip failed: escape(%q) = %q, unescape = %q",
					input, escaped, unescaped)
			}
		})
	}
}

func TestRunBasic(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		stdin    string
		expected string
		exitCode int
	}{
		{
			name:     "simple argument",
			args:     []string{"hello world"},
			expected: "hello world\n",
			exitCode: 0,
		},
		{
			name:     "escape quotes",
			args:     []string{`hello "world"`},
			expected: `hello \"world\"` + "\n",
			exitCode: 0,
		},
		{
			name:     "multiple arguments",
			args:     []string{"one", "two", "three"},
			expected: "one\ntwo\nthree\n",
			exitCode: 0,
		},
		{
			name:     "unescape mode",
			args:     []string{"-u", `hello\nworld`},
			expected: "hello\nworld\n",
			exitCode: 0,
		},
		{
			name:     "wrap quotes",
			args:     []string{"-q", "hello"},
			expected: "\"hello\"\n",
			exitCode: 0,
		},
		{
			name:     "raw output",
			args:     []string{"-r", "hello"},
			expected: "hello",
			exitCode: 0,
		},
		{
			name:     "stdin input",
			args:     []string{},
			stdin:    "hello world",
			expected: "hello world\n",
			exitCode: 0,
		},
		{
			name:     "line mode",
			args:     []string{"-l"},
			stdin:    "line1\nline2\nline3",
			expected: "line1\nline2\nline3\n",
			exitCode: 0,
		},
		{
			name:     "ascii mode",
			args:     []string{"-a", "日本語"},
			expected: `\u65e5\u672c\u8a9e` + "\n",
			exitCode: 0,
		},
		{
			name:     "html safe mode",
			args:     []string{"--html-safe", "<b>"},
			expected: `\u003cb\u003e` + "\n",
			exitCode: 0,
		},
		{
			name:     "help flag",
			args:     []string{"--help"},
			expected: "", // just check no error
			exitCode: 0,
		},
		{
			name:     "version flag",
			args:     []string{"--version"},
			expected: "", // just check no error
			exitCode: 0,
		},
		{
			name:     "unknown option",
			args:     []string{"--unknown"},
			expected: "",
			exitCode: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			stdin := strings.NewReader(tt.stdin)

			exitCode := run(tt.args, stdin, &stdout, &stderr)

			if exitCode != tt.exitCode {
				t.Errorf("exit code = %d, want %d (stderr: %s)",
					exitCode, tt.exitCode, stderr.String())
			}

			if tt.expected != "" && stdout.String() != tt.expected {
				t.Errorf("stdout = %q, want %q", stdout.String(), tt.expected)
			}
		})
	}
}

func TestNullDelimited(t *testing.T) {
	var stdout, stderr bytes.Buffer
	stdin := strings.NewReader("one\x00two\x00three\x00")

	exitCode := run([]string{"-0"}, stdin, &stdout, &stderr)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}

	expected := "one\ntwo\nthree\n"
	if stdout.String() != expected {
		t.Errorf("stdout = %q, want %q", stdout.String(), expected)
	}
}

func TestCombinedFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer

	exitCode := run([]string{"-qr", "test"}, strings.NewReader(""), &stdout, &stderr)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}

	expected := `"test"`
	if stdout.String() != expected {
		t.Errorf("stdout = %q, want %q", stdout.String(), expected)
	}
}

func TestDoubleDash(t *testing.T) {
	var stdout, stderr bytes.Buffer

	// -- should stop flag parsing, so -u should be treated as a string
	exitCode := run([]string{"--", "-u"}, strings.NewReader(""), &stdout, &stderr)

	if exitCode != 0 {
		t.Errorf("exit code = %d, want 0", exitCode)
	}

	expected := "-u\n"
	if stdout.String() != expected {
		t.Errorf("stdout = %q, want %q", stdout.String(), expected)
	}
}

func TestParseArgsErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"file without value", []string{"--file"}},
		{"output without value", []string{"--output"}},
		{"short file without value", []string{"-f"}},
		{"short output without value", []string{"-o"}},
		{"strict and replace", []string{"--strict", "--replace"}},
		{"null and lines", []string{"--null", "--lines"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseArgs(tt.args)
			if err == nil {
				t.Errorf("parseArgs(%v) expected error, got nil", tt.args)
			}
		})
	}
}

func TestCompletionGeneration(t *testing.T) {
	shells := []string{"bash", "zsh", "fish"}

	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := run([]string{"--completion", shell}, strings.NewReader(""), &stdout, &stderr)

			if exitCode != 0 {
				t.Errorf("exit code = %d, want 0", exitCode)
			}

			if stdout.Len() == 0 {
				t.Error("expected completion output, got empty")
			}
		})
	}

	// Unknown shell
	var stdout, stderr bytes.Buffer
	exitCode := run([]string{"--completion", "unknown"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 2 {
		t.Errorf("exit code = %d, want 2 for unknown shell", exitCode)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
