package gitutil

import (
	"os"
	"strings"
)

func Environment() []string {
	local := map[string]bool{
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": true,
		"GIT_COMMON_DIR":                   true,
		"GIT_CONFIG":                       true,
		"GIT_CONFIG_COUNT":                 true,
		"GIT_CONFIG_PARAMETERS":            true,
		"GIT_DIR":                          true,
		"GIT_GRAFT_FILE":                   true,
		"GIT_IMPLICIT_WORK_TREE":           true,
		"GIT_INDEX_FILE":                   true,
		"GIT_NO_REPLACE_OBJECTS":           true,
		"GIT_OBJECT_DIRECTORY":             true,
		"GIT_PREFIX":                       true,
		"GIT_REPLACE_REF_BASE":             true,
		"GIT_SHALLOW_FILE":                 true,
		"GIT_WORK_TREE":                    true,
	}
	environment := make([]string, 0, len(os.Environ()))
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if !local[name] {
			environment = append(environment, value)
		}
	}
	return environment
}
