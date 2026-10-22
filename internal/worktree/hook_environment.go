package worktree

import "strings"

// Ticket context belongs to hooks, not the persisted .env.worktree environment.
func ticketHookEnvironment(environment, ticket map[string]string) map[string]string {
	result := make(map[string]string, len(environment)+3)
	for name, value := range environment {
		result[name] = value
	}
	for _, field := range []string{"identifier", "title", "url"} {
		result["HWT_TICKET_"+strings.ToUpper(field)] = ticket[field]
	}
	return result
}

func hookEnvironmentList(environment map[string]string) []string {
	var result []string
	for _, entry := range environmentList(environment) {
		name, _, _ := strings.Cut(entry, "=")
		_, provided := environment[name]
		if !strings.HasPrefix(name, "HWT_TICKET_") || provided {
			result = append(result, entry)
		}
	}
	return result
}
