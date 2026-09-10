package previewurl

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var placeholders = map[string]bool{
	"repository":       true,
	"branch":           true,
	"sanitized_branch": true,
	"worktree":         true,
}

func ValidateTemplate(template string) error {
	_, err := walk(template, func(name string) (string, error) {
		if !placeholders[name] {
			return "", fmt.Errorf("unknown placeholder {%s}; supported placeholders are {repository}, {branch}, {sanitized_branch}, and {worktree}", name)
		}
		return "value", nil
	})
	return err
}

func Expand(template string, values map[string]string) (string, error) {
	result, err := walk(template, func(name string) (string, error) {
		if !placeholders[name] {
			return "", fmt.Errorf("unknown placeholder {%s}; supported placeholders are {repository}, {branch}, {sanitized_branch}, and {worktree}", name)
		}
		value := values[name]
		if value == "" {
			return "", fmt.Errorf("placeholder {%s} is unavailable in this context", name)
		}
		return escape(value), nil
	})
	if err != nil {
		return "", err
	}
	return result, nil
}

func SanitizeBranch(branch string) string {
	var result strings.Builder
	separator := false
	for _, character := range branch {
		if character >= 'A' && character <= 'Z' {
			character += 'a' - 'A'
		}
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			if separator && result.Len() > 0 {
				if result.Len() >= 62 {
					break
				}
				result.WriteByte('-')
			}
			if result.Len() >= 63 {
				break
			}
			result.WriteRune(character)
			separator = false
		} else {
			separator = true
		}
	}
	return strings.TrimRight(result.String(), "-")
}

func walk(template string, replace func(string) (string, error)) (string, error) {
	if strings.TrimSpace(template) == "" {
		return "", errors.New("template cannot be empty")
	}
	var result strings.Builder
	for position := 0; position < len(template); {
		start := strings.IndexByte(template[position:], '{')
		end := strings.IndexByte(template[position:], '}')
		if start < 0 {
			if end >= 0 {
				return "", errors.New("unexpected } in template")
			}
			result.WriteString(template[position:])
			break
		}
		start += position
		if end >= 0 && position+end < start {
			return "", errors.New("unexpected } in template")
		}
		result.WriteString(template[position:start])
		closeOffset := strings.IndexByte(template[start+1:], '}')
		if closeOffset < 0 {
			return "", errors.New("unclosed { in template")
		}
		close := start + 1 + closeOffset
		name := template[start+1 : close]
		if name == "" || strings.ContainsRune(name, '{') {
			return "", fmt.Errorf("invalid placeholder %q", template[start:close+1])
		}
		value, err := replace(name)
		if err != nil {
			return "", err
		}
		result.WriteString(value)
		position = close + 1
	}
	value := result.String()
	if err := validateURL(value); err != nil {
		return "", err
	}
	return value, nil
}

func validateURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("template does not produce a valid URL: %w", err)
	}
	if (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return errors.New("template must produce an absolute http or https URL")
	}
	return nil
}

func escape(value string) string {
	const hexadecimal = "0123456789ABCDEF"
	var result strings.Builder
	for _, character := range []byte(value) {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("-._~", rune(character)) {
			result.WriteByte(character)
		} else {
			result.WriteByte('%')
			result.WriteByte(hexadecimal[character>>4])
			result.WriteByte(hexadecimal[character&15])
		}
	}
	return result.String()
}
