// jsonescape - A robust CLI tool for escaping and unescaping JSON strings
package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"unicode/utf8"
)

const (
	version = "1.0.0"
	name    = "jsonescape"
)

// Exit codes
const (
	exitSuccess    = 0
	exitError      = 1
	exitUsageError = 2
)

// Config holds all CLI configuration options
type Config struct {
	// Input options
	InputFiles    []string
	ReadStdin     bool
	NullDelimited bool
	LineMode      bool

	// Output options
	Unescape   bool
	WrapQuotes bool
	RawOutput  bool
	OutputFile string

	// Encoding options
	ASCIIOnly  bool
	HTMLSafe   bool
	StrictUTF8 bool
	ReplaceUTF8 bool

	// Meta options
	ShowHelp       bool
	ShowVersion    bool
	GenerateCompletion string

	// Positional args (strings to process)
	Args []string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	config, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		fmt.Fprintf(stderr, "Try '%s --help' for more information.\n", name)
		return exitUsageError
	}

	if config.ShowHelp {
		printHelp(stdout)
		return exitSuccess
	}

	if config.ShowVersion {
		fmt.Fprintf(stdout, "%s version %s (%s/%s)\n", name, version, runtime.GOOS, runtime.GOARCH)
		return exitSuccess
	}

	if config.GenerateCompletion != "" {
		return generateCompletion(config.GenerateCompletion, stdout, stderr)
	}

	// Determine output writer
	var output io.Writer = stdout
	if config.OutputFile != "" {
		f, err := os.Create(config.OutputFile)
		if err != nil {
			fmt.Fprintf(stderr, "Error: cannot create output file: %v\n", err)
			return exitError
		}
		defer f.Close()
		output = f
	}

	// Create the processor
	proc := &Processor{
		Config: config,
		Output: output,
		Stderr: stderr,
	}

	// Determine input sources and process
	hasInput := false

	// Process positional arguments first
	for _, arg := range config.Args {
		hasInput = true
		if err := proc.ProcessString(arg); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return exitError
		}
	}

	// Process input files
	for _, path := range config.InputFiles {
		hasInput = true
		if err := proc.ProcessFile(path); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return exitError
		}
	}

	// Process stdin if explicitly requested or if no other input and stdin is piped
	if config.ReadStdin || (!hasInput && !isTerminal(stdin)) {
		if err := proc.ProcessReader(stdin); err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return exitError
		}
		hasInput = true
	}

	// No input provided
	if !hasInput {
		fmt.Fprintf(stderr, "Error: no input provided\n")
		fmt.Fprintf(stderr, "Try '%s --help' for more information.\n", name)
		return exitUsageError
	}

	return exitSuccess
}

// Processor handles the actual escaping/unescaping
type Processor struct {
	Config *Config
	Output io.Writer
	Stderr io.Writer
	count  int // number of items processed
}

// ProcessString processes a single string argument
func (p *Processor) ProcessString(s string) error {
	return p.processItem(s)
}

// ProcessFile processes input from a file
func (p *Processor) ProcessFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open file %q: %w", path, err)
	}
	defer f.Close()
	return p.ProcessReader(f)
}

// ProcessReader processes input from a reader
func (p *Processor) ProcessReader(r io.Reader) error {
	if p.Config.NullDelimited {
		return p.processNullDelimited(r)
	}
	if p.Config.LineMode {
		return p.processLines(r)
	}
	// Default: read entire input as one string
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	// Trim trailing newline for convenience (common when piping)
	s := string(data)
	s = strings.TrimSuffix(s, "\n")
	s = strings.TrimSuffix(s, "\r")
	return p.processItem(s)
}

func (p *Processor) processLines(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	// Use a larger buffer for long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024) // 10MB max line size

	for scanner.Scan() {
		if err := p.processItem(scanner.Text()); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func (p *Processor) processNullDelimited(r io.Reader) error {
	reader := bufio.NewReader(r)
	for {
		item, err := reader.ReadString('\x00')
		if err != nil && err != io.EOF {
			return fmt.Errorf("reading input: %w", err)
		}
		// Remove the null terminator if present
		item = strings.TrimSuffix(item, "\x00")
		
		if item != "" || err == nil {
			if err := p.processItem(item); err != nil {
				return err
			}
		}
		
		if err == io.EOF {
			break
		}
	}
	return nil
}

func (p *Processor) processItem(s string) error {
	// Validate UTF-8 if strict mode
	if p.Config.StrictUTF8 && !utf8.ValidString(s) {
		return errors.New("input contains invalid UTF-8")
	}

	// Replace invalid UTF-8 if requested
	if p.Config.ReplaceUTF8 {
		s = strings.ToValidUTF8(s, "\uFFFD")
	}

	var result string
	var err error

	if p.Config.Unescape {
		result, err = jsonUnescape(s)
		if err != nil {
			return fmt.Errorf("unescaping: %w", err)
		}
	} else {
		result = jsonEscape(s, p.Config.ASCIIOnly, p.Config.HTMLSafe)
	}

	// Wrap in quotes if requested
	if p.Config.WrapQuotes {
		result = `"` + result + `"`
	}

	// Output
	if p.Config.RawOutput {
		fmt.Fprint(p.Output, result)
	} else {
		fmt.Fprintln(p.Output, result)
	}

	p.count++
	return nil
}

// jsonEscape escapes a string for use in JSON
func jsonEscape(s string, asciiOnly, htmlSafe bool) string {
	var buf bytes.Buffer
	buf.Grow(len(s) + 10) // Pre-allocate with some headroom

	for _, r := range s {
		switch r {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		case '<':
			if htmlSafe {
				buf.WriteString(`\u003c`)
			} else {
				buf.WriteRune(r)
			}
		case '>':
			if htmlSafe {
				buf.WriteString(`\u003e`)
			} else {
				buf.WriteRune(r)
			}
		case '&':
			if htmlSafe {
				buf.WriteString(`\u0026`)
			} else {
				buf.WriteRune(r)
			}
		default:
			// Control characters (U+0000 through U+001F) must be escaped
			if r < 0x20 {
				fmt.Fprintf(&buf, `\u%04x`, r)
			} else if asciiOnly && r > 127 {
				// Escape non-ASCII characters
				if r <= 0xFFFF {
					fmt.Fprintf(&buf, `\u%04x`, r)
				} else {
					// Use surrogate pairs for characters outside BMP
					r1, r2 := utf16Surrogates(r)
					fmt.Fprintf(&buf, `\u%04x\u%04x`, r1, r2)
				}
			} else {
				buf.WriteRune(r)
			}
		}
	}

	return buf.String()
}

// utf16Surrogates returns the UTF-16 surrogate pair for a rune outside the BMP
func utf16Surrogates(r rune) (rune, rune) {
	r -= 0x10000
	return 0xD800 + (r>>10)&0x3FF, 0xDC00 + r&0x3FF
}

// jsonUnescape unescapes a JSON string
func jsonUnescape(s string) (string, error) {
	var buf bytes.Buffer
	buf.Grow(len(s))

	i := 0
	for i < len(s) {
		if s[i] != '\\' {
			buf.WriteByte(s[i])
			i++
			continue
		}

		// Handle escape sequence
		if i+1 >= len(s) {
			return "", errors.New("incomplete escape sequence at end of string")
		}

		i++ // skip the backslash
		switch s[i] {
		case '"':
			buf.WriteByte('"')
		case '\\':
			buf.WriteByte('\\')
		case '/':
			buf.WriteByte('/')
		case 'b':
			buf.WriteByte('\b')
		case 'f':
			buf.WriteByte('\f')
		case 'n':
			buf.WriteByte('\n')
		case 'r':
			buf.WriteByte('\r')
		case 't':
			buf.WriteByte('\t')
		case 'u':
			// Unicode escape: \uXXXX
			if i+4 >= len(s) {
				return "", errors.New("incomplete unicode escape sequence")
			}
			hex := s[i+1 : i+5]
			r, err := parseHexRune(hex)
			if err != nil {
				return "", fmt.Errorf("invalid unicode escape \\u%s: %w", hex, err)
			}
			
			// Check for surrogate pair
			if r >= 0xD800 && r <= 0xDBFF {
				// High surrogate - look for low surrogate
				if i+10 < len(s) && s[i+5] == '\\' && s[i+6] == 'u' {
					hex2 := s[i+7 : i+11]
					r2, err := parseHexRune(hex2)
					if err == nil && r2 >= 0xDC00 && r2 <= 0xDFFF {
						// Valid surrogate pair
						combined := 0x10000 + (rune(r)-0xD800)*0x400 + (rune(r2) - 0xDC00)
						buf.WriteRune(combined)
						i += 11 // skip past \uXXXX\uXXXX (will be incremented to 12 at end of loop)
						continue
					}
				}
			}
			
			buf.WriteRune(r)
			i += 4
		default:
			return "", fmt.Errorf("invalid escape sequence \\%c", s[i])
		}
		i++
	}

	return buf.String(), nil
}

func parseHexRune(hex string) (rune, error) {
	var r rune
	for _, c := range hex {
		r <<= 4
		switch {
		case c >= '0' && c <= '9':
			r |= rune(c - '0')
		case c >= 'a' && c <= 'f':
			r |= rune(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			r |= rune(c - 'A' + 10)
		default:
			return 0, fmt.Errorf("invalid hex character %q", c)
		}
	}
	return r, nil
}

// parseArgs parses command-line arguments
func parseArgs(args []string) (*Config, error) {
	config := &Config{}

	i := 0
	for i < len(args) {
		arg := args[i]

		// Stop processing flags after --
		if arg == "--" {
			config.Args = append(config.Args, args[i+1:]...)
			break
		}

		// Long options
		if strings.HasPrefix(arg, "--") {
			name, value, hasValue := strings.Cut(arg[2:], "=")
			
			switch name {
			case "help":
				config.ShowHelp = true
			case "version":
				config.ShowVersion = true
			case "unescape":
				config.Unescape = true
			case "quote":
				config.WrapQuotes = true
			case "raw":
				config.RawOutput = true
			case "null":
				config.NullDelimited = true
			case "lines":
				config.LineMode = true
			case "ascii":
				config.ASCIIOnly = true
			case "html-safe":
				config.HTMLSafe = true
			case "strict":
				config.StrictUTF8 = true
			case "replace":
				config.ReplaceUTF8 = true
			case "stdin":
				config.ReadStdin = true
			case "file":
				if !hasValue {
					i++
					if i >= len(args) {
						return nil, errors.New("--file requires a value")
					}
					value = args[i]
				}
				config.InputFiles = append(config.InputFiles, value)
			case "output":
				if !hasValue {
					i++
					if i >= len(args) {
						return nil, errors.New("--output requires a value")
					}
					value = args[i]
				}
				config.OutputFile = value
			case "completion":
				if !hasValue {
					i++
					if i >= len(args) {
						return nil, errors.New("--completion requires a shell name (bash, zsh, fish)")
					}
					value = args[i]
				}
				config.GenerateCompletion = value
			default:
				return nil, fmt.Errorf("unknown option: --%s", name)
			}
			i++
			continue
		}

		// Short options
		if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			for j := 1; j < len(arg); j++ {
				c := arg[j]
				switch c {
				case 'h':
					config.ShowHelp = true
				case 'V':
					config.ShowVersion = true
				case 'u':
					config.Unescape = true
				case 'q':
					config.WrapQuotes = true
				case 'r':
					config.RawOutput = true
				case '0':
					config.NullDelimited = true
				case 'l':
					config.LineMode = true
				case 'a':
					config.ASCIIOnly = true
				case 's':
					config.StrictUTF8 = true
				case 'f':
					// -f requires a value
					if j+1 < len(arg) {
						config.InputFiles = append(config.InputFiles, arg[j+1:])
						j = len(arg) // end inner loop
					} else {
						i++
						if i >= len(args) {
							return nil, errors.New("-f requires a value")
						}
						config.InputFiles = append(config.InputFiles, args[i])
					}
				case 'o':
					// -o requires a value
					if j+1 < len(arg) {
						config.OutputFile = arg[j+1:]
						j = len(arg) // end inner loop
					} else {
						i++
						if i >= len(args) {
							return nil, errors.New("-o requires a value")
						}
						config.OutputFile = args[i]
					}
				default:
					return nil, fmt.Errorf("unknown option: -%c", c)
				}
			}
			i++
			continue
		}

		// Positional argument
		config.Args = append(config.Args, arg)
		i++
	}

	// Validate conflicting options
	if config.StrictUTF8 && config.ReplaceUTF8 {
		return nil, errors.New("--strict and --replace are mutually exclusive")
	}
	if config.NullDelimited && config.LineMode {
		return nil, errors.New("--null and --lines are mutually exclusive")
	}

	return config, nil
}

// isTerminal attempts to detect if the reader is a terminal
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func printHelp(w io.Writer) {
	help := `Usage: %s [OPTIONS] [STRING...]

A robust CLI tool for escaping and unescaping JSON strings.

Arguments:
  [STRING...]              Strings to process (if not reading from stdin/file)

Input Options:
  -f, --file <PATH>        Read input from file (can be used multiple times)
      --stdin              Explicitly read from stdin
  -l, --lines              Process each line as a separate string
  -0, --null               Input is null-delimited (like xargs -0)

Output Options:
  -u, --unescape           Unescape JSON string instead of escaping
  -q, --quote              Wrap output in double quotes
  -r, --raw                Don't add trailing newline to output
  -o, --output <PATH>      Write output to file instead of stdout

Encoding Options:
  -a, --ascii              Escape all non-ASCII characters as \uXXXX
      --html-safe          Also escape <, >, & for HTML embedding
  -s, --strict             Reject invalid UTF-8 input
      --replace            Replace invalid UTF-8 with replacement character

Other Options:
  -h, --help               Show this help message
  -V, --version            Show version information
      --completion <SHELL> Generate shell completion (bash, zsh, fish)

Examples:
  # Escape a string from argument
  %s 'Hello "World"'
  
  # Escape piped input
  echo 'Line 1\nLine 2' | %s
  
  # Process multiple lines
  cat file.txt | %s --lines
  
  # Unescape a JSON string
  %s -u 'Hello\nWorld'
  
  # Escape for HTML embedding
  %s --html-safe '<script>alert("XSS")</script>'
  
  # ASCII-only output (useful for legacy systems)
  %s --ascii '日本語'
  
  # Process null-delimited input (handles strings with newlines)
  find . -print0 | %s -0

Exit Codes:
  0    Success
  1    Error during processing
  2    Invalid usage
`
	fmt.Fprintf(w, help, name, name, name, name, name, name, name, name)
}

func generateCompletion(shell string, stdout, stderr io.Writer) int {
	switch strings.ToLower(shell) {
	case "bash":
		fmt.Fprint(stdout, bashCompletion)
	case "zsh":
		fmt.Fprint(stdout, zshCompletion)
	case "fish":
		fmt.Fprint(stdout, fishCompletion)
	default:
		fmt.Fprintf(stderr, "Error: unknown shell %q (supported: bash, zsh, fish)\n", shell)
		return exitUsageError
	}
	return exitSuccess
}

var bashCompletion = `# bash completion for jsonescape
_jsonescape() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    opts="-h --help -V --version -u --unescape -q --quote -r --raw -f --file -o --output -l --lines -0 --null -a --ascii --html-safe -s --strict --replace --stdin --completion"

    case "${prev}" in
        -f|--file|-o|--output)
            COMPREPLY=( $(compgen -f -- "${cur}") )
            return 0
            ;;
        --completion)
            COMPREPLY=( $(compgen -W "bash zsh fish" -- "${cur}") )
            return 0
            ;;
    esac

    if [[ ${cur} == -* ]]; then
        COMPREPLY=( $(compgen -W "${opts}" -- "${cur}") )
        return 0
    fi
}
complete -F _jsonescape jsonescape
`

var zshCompletion = `#compdef jsonescape

_jsonescape() {
    _arguments \
        '-h[Show help]' \
        '--help[Show help]' \
        '-V[Show version]' \
        '--version[Show version]' \
        '-u[Unescape mode]' \
        '--unescape[Unescape mode]' \
        '-q[Wrap in quotes]' \
        '--quote[Wrap in quotes]' \
        '-r[Raw output]' \
        '--raw[Raw output]' \
        '-f[Input file]:file:_files' \
        '--file[Input file]:file:_files' \
        '-o[Output file]:file:_files' \
        '--output[Output file]:file:_files' \
        '-l[Line mode]' \
        '--lines[Line mode]' \
        '-0[Null-delimited input]' \
        '--null[Null-delimited input]' \
        '-a[ASCII only]' \
        '--ascii[ASCII only]' \
        '--html-safe[HTML safe escaping]' \
        '-s[Strict UTF-8]' \
        '--strict[Strict UTF-8]' \
        '--replace[Replace invalid UTF-8]' \
        '--stdin[Read from stdin]' \
        '--completion[Generate completion]:shell:(bash zsh fish)'
}
`

var fishCompletion = `# fish completion for jsonescape
complete -c jsonescape -s h -l help -d 'Show help'
complete -c jsonescape -s V -l version -d 'Show version'
complete -c jsonescape -s u -l unescape -d 'Unescape mode'
complete -c jsonescape -s q -l quote -d 'Wrap in quotes'
complete -c jsonescape -s r -l raw -d 'Raw output (no trailing newline)'
complete -c jsonescape -s f -l file -r -d 'Input file'
complete -c jsonescape -s o -l output -r -d 'Output file'
complete -c jsonescape -s l -l lines -d 'Process each line separately'
complete -c jsonescape -s 0 -l null -d 'Null-delimited input'
complete -c jsonescape -s a -l ascii -d 'Escape non-ASCII as \\uXXXX'
complete -c jsonescape -l html-safe -d 'Escape <, >, & for HTML'
complete -c jsonescape -s s -l strict -d 'Reject invalid UTF-8'
complete -c jsonescape -l replace -d 'Replace invalid UTF-8'
complete -c jsonescape -l stdin -d 'Read from stdin'
complete -c jsonescape -l completion -xa 'bash zsh fish' -d 'Generate shell completion'
`