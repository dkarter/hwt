package urltemplate

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

func ValidateTemplate(template string) error {
	_, err := parse(template, func(string) (string, error) { return "value", nil }, true)
	return err
}

func Placeholders(template string) ([]string, error) {
	return placeholders(template, true)
}

func PlaceholdersRaw(template string) ([]string, error) {
	return placeholders(template, false)
}

func placeholders(template string, validate bool) ([]string, error) {
	seen := map[string]bool{}
	var names []string
	_, err := parse(template, func(name string) (string, error) {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
		return "value", nil
	}, validate)
	return names, err
}

func Expand(template string, values map[string]string) (string, error) {
	return expand(template, values, true)
}

func ExpandRaw(template string, values map[string]string) (string, error) {
	return expand(template, values, false)
}

func expand(template string, values map[string]string, encode bool) (string, error) {
	return parse(template, func(name string) (string, error) {
		value, exists := values[name]
		if !exists {
			return "", fmt.Errorf("unknown placeholder {%s}", name)
		}
		if value == "" {
			return "", fmt.Errorf("placeholder {%s} has no value", name)
		}
		if encode {
			value = escape(value)
		}
		return value, nil
	}, encode)
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

func parse(template string, replace func(string) (string, error), validate bool) (string, error) {
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
		if !ValidPlaceholder(name) {
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
	if validate {
		if err := validateURL(value); err != nil {
			return "", err
		}
	}
	return value, nil
}

func ValidPlaceholder(name string) bool {
	for _, part := range strings.Split(name, ".") {
		if part == "" {
			return false
		}
		for position, character := range part {
			if !(character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character == '_' || position > 0 && (character >= '0' && character <= '9' || character == '-')) {
				return false
			}
		}
	}
	return true
}

func validateURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("template does not produce a valid URL: %w", err)
	}
	if parsed.Scheme == "" {
		return errors.New("template must produce an absolute URL with a scheme")
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
