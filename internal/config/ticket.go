package config

import (
	"fmt"
	"strings"

	"github.com/dkarter/hwt/internal/urltemplate"
)

type TicketCommand struct {
	Command []string            `json:"command" yaml:"command"`
	Output  TicketCommandOutput `json:"output,omitempty" yaml:"output,omitempty"`
}

type TicketCommandOutput struct {
	Branch   string            `json:"branch,omitempty" yaml:"branch,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

func validateTicketCommands(commands map[string]TicketCommand) error {
	for name, ticket := range commands {
		if !validServiceName(name) {
			return fmt.Errorf("ticket command name %q must start with a letter and contain only letters, numbers, underscores, or hyphens", name)
		}
		if err := validateArgv(fmt.Sprintf("ticket_commands.%s.command", name), ticket.Command); err != nil {
			return err
		}
		if len(ticket.Command) > 0 && strings.Contains(ticket.Command[0], "{input}") {
			return fmt.Errorf("ticket_commands.%s.command executable cannot be {input}", name)
		}
		branch := ticket.Output.Branch
		if branch == "" {
			branch = "branchName"
		}
		if !urltemplate.ValidPlaceholder(branch) {
			return fmt.Errorf("ticket_commands.%s.output.branch %q must be a dot-separated JSON field selector", name, branch)
		}
		for key, selector := range ticket.Output.Metadata {
			if !urltemplate.ValidPlaceholder(key) {
				return fmt.Errorf("ticket_commands.%s.output.metadata key %q must be a dot-separated identifier", name, key)
			}
			if !urltemplate.ValidPlaceholder(selector) {
				return fmt.Errorf("ticket_commands.%s.output.metadata.%s %q must be a dot-separated JSON field selector", name, key, selector)
			}
		}
	}
	return nil
}

func mergeTicketCommands(global, project *map[string]TicketCommand) map[string]TicketCommand {
	result := map[string]TicketCommand{}
	for _, source := range []*map[string]TicketCommand{global, project} {
		if source == nil {
			continue
		}
		for name, ticket := range *source {
			copy := ticket
			if copy.Output.Branch == "" {
				copy.Output.Branch = "branchName"
			}
			copy.Command = append([]string(nil), ticket.Command...)
			if ticket.Output.Metadata != nil {
				copy.Output.Metadata = make(map[string]string, len(ticket.Output.Metadata))
				for key, selector := range ticket.Output.Metadata {
					copy.Output.Metadata[key] = selector
				}
			}
			result[name] = copy
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
