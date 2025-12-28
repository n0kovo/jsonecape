# jsonescape

A simple CLI tool for escaping and unescaping JSON strings.

## Installation

```bash
go install github.com/n0kovo/jsonescape@latest
```

Or build from source:

```bash
git clone https://github.com/n0kovo/jsonescape
cd jsonescape
go build -o jsonescape .
```

## Usage

```bash
# Escape a string
jsonescape 'Hello "World"'
# Output: Hello \"World\"

# Pipe something in
echo 'Line 1
Line 2' | jsonescape
# Output: Line 1\nLine 2

# Unescape
jsonescape -u 'Hello\nWorld'
# Output:
# Hello
# World

# Wrap output in quotes (useful for inserting into JSON)
jsonescape -q "some value"
# Output: "some value"
```

## Options

```yaml
Input:
  -f, --file <PATH>   Read from file (repeatable)
  --stdin             Force reading from stdin
  -l, --lines         Treat each line as separate input
  -0, --null          Null-delimited input (for xargs -0 style)

Output:
  -u, --unescape      Reverse the operation
  -q, --quote         Wrap output in double quotes
  -r, --raw           No trailing newline
  -o, --output <PATH> Write to file

Encoding:
  -a, --ascii         Escape non-ASCII as \uXXXX
  --html-safe         Also escape <, >, &
  -s, --strict        Fail on invalid UTF-8
  --replace           Replace invalid UTF-8 with �

Other:
  -h, --help
  -V, --version
  --completion <SHELL>  Generate completions (bash, zsh, fish)
```

## Examples

**Escape filenames with special characters:**

```bash
find . -name "*.json" -print0 | jsonescape -0
```

**Process a file line by line:**

```bash
jsonescape -l -f input.txt -o output.txt
```

**Make output safe for embedding in HTML:**

```bash
jsonescape --html-safe '<script>alert("hi")</script>'
# Output: \u003cscript\u003ealert(\"hi\")\u003c/script\u003e
```

**ASCII-only output for legacy systems:**

```bash
jsonescape -a '日本語'
# Output: \u65e5\u672c\u8a9e
```

**Use in a shell script:**

```bash
value=$(jsonescape -qr "$user_input")
echo "{\"name\": $value}" > output.json
```

## Exit Codes

- `0` - Success
- `1` - Error during processing
- `2` - Bad usage (unknown flag, missing argument, etc.)

## Shell Completions

```bash
# Bash
jsonescape --completion bash > /etc/bash_completion.d/jsonescape

# Zsh
jsonescape --completion zsh > ~/.zsh/completions/_jsonescape

# Fish
jsonescape --completion fish > ~/.config/fish/completions/jsonescape.fish
```

## Notes

- Stdin is read automatically if no arguments are given and input is piped
- Trailing newlines are stripped from stdin input (usually what you want)
- Surrogate pairs are handled correctly for emoji and other characters outside the BMP
- No external dependencies
