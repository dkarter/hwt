package worktree

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/dkarter/hwt/internal/config"
)

func runTicketCommand(cwd string, ticket config.TicketCommand, input string) (ticketResult, error) {
	arguments, err := ticketCommandArguments(ticket.Command[1:], input)
	if err != nil {
		return ticketResult{}, err
	}
	cmd := exec.Command(ticket.Command[0], arguments...)
	cmd.Dir = cwd
	cmd.Stdin = os.Stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	if strings.TrimSpace(input) == "" {
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
	} else {
		cmd.Stderr = &stderr
	}
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return ticketResult{}, fmt.Errorf("find ticket command %q: %w", ticket.Command[0], err)
		}
		message := strings.TrimSpace(stderr.String())
		if strings.TrimSpace(input) == "" && message != "" {
			message = ""
		}
		if message == "" && strings.TrimSpace(stderr.String()) == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message != "" {
			return ticketResult{}, fmt.Errorf("ticket command %q failed: %s: %w", ticket.Command[0], message, err)
		}
		return ticketResult{}, fmt.Errorf("ticket command %q failed: %w", ticket.Command[0], err)
	}

	var output any
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		return ticketResult{}, fmt.Errorf("decode ticket command JSON output: %w", err)
	}

	branchSelector := ticket.Output.Branch
	if branchSelector == "" {
		branchSelector = "branchName"
	}
	branch, err := ticketOutputString(output, branchSelector)
	if err != nil {
		return ticketResult{}, fmt.Errorf("ticket command JSON output branch selector %q: %w", branchSelector, err)
	}
	if strings.TrimSpace(branch) == "" {
		return ticketResult{}, fmt.Errorf("ticket command JSON output branch selector %q returned an empty string", branchSelector)
	}

	result := ticketResult{BranchName: branch}
	if ticket.Output.Metadata != nil {
		result.Metadata = make(map[string]string, len(ticket.Output.Metadata))
		for key, selector := range ticket.Output.Metadata {
			value, err := ticketOutputString(output, selector)
			if err != nil {
				return ticketResult{}, fmt.Errorf("ticket command JSON output metadata selector %q for %q: %w", selector, key, err)
			}
			result.Metadata[key] = value
		}
	} else if object, ok := output.(map[string]any); ok {
		if metadata, exists := object["metadata"]; exists && metadata != nil {
			values, ok := metadata.(map[string]any)
			if !ok {
				return ticketResult{}, errors.New("ticket command JSON output metadata must be an object")
			}
			result.Metadata = make(map[string]string, len(values))
			for key, value := range values {
				stringValue, ok := value.(string)
				if !ok {
					return ticketResult{}, fmt.Errorf("ticket command JSON output metadata value %q must be a string", key)
				}
				result.Metadata[key] = stringValue
			}
		}
	}
	if err := validateTicketMetadata(result.Metadata); err != nil {
		return ticketResult{}, fmt.Errorf("ticket command JSON output metadata: %w", err)
	}
	return result, nil
}

func ticketCommandArguments(configured []string, input string) ([]string, error) {
	hasInput := strings.TrimSpace(input) != ""
	arguments := make([]string, 0, len(configured))
	for _, argument := range configured {
		if argument == "{input}" && !hasInput {
			continue
		}
		if strings.Contains(argument, "{input}") {
			if !hasInput {
				return nil, fmt.Errorf("ticket command argument %q requires a positional input", argument)
			}
			argument = strings.ReplaceAll(argument, "{input}", input)
		}
		arguments = append(arguments, argument)
	}
	return arguments, nil
}

func ticketOutputString(output any, selector string) (string, error) {
	current := output
	for _, field := range strings.Split(selector, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return "", fmt.Errorf("field %q is not inside a JSON object", field)
		}
		next, exists := object[field]
		if !exists {
			return "", fmt.Errorf("field %q is missing", field)
		}
		current = next
	}
	value, ok := current.(string)
	if !ok {
		return "", errors.New("selected value must be a string")
	}
	return value, nil
}
