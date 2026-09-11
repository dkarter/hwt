package worktree

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dkarter/hwt/internal/urltemplate"
)

const ticketMetadataName = "hwt-ticket-metadata-v1.json"

func ReadTicketMetadata(cwd string) (map[string]string, error) {
	gitDir, commonDir, _, err := metadata(cwd)
	if err != nil {
		return nil, fmt.Errorf("resolve ticket metadata worktree: %w", err)
	}
	if samePath(gitDir, commonDir) {
		return nil, nil
	}
	data, err := os.ReadFile(filepath.Join(gitDir, ticketMetadataName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read ticket metadata: %w", err)
	}
	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("decode ticket metadata: %w", err)
	}
	if err := validateTicketMetadata(values); err != nil {
		return nil, fmt.Errorf("validate ticket metadata: %w", err)
	}
	return values, nil
}

func writeTicketMetadata(cwd string, values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	if err := validateTicketMetadata(values); err != nil {
		return err
	}
	gitDir, commonDir, _, err := metadata(cwd)
	if err != nil {
		return fmt.Errorf("resolve ticket metadata worktree: %w", err)
	}
	if samePath(gitDir, commonDir) {
		return errors.New("ticket metadata can only be stored for a linked worktree")
	}
	temporary, err := os.CreateTemp(gitDir, "."+ticketMetadataName+".*")
	if err != nil {
		return fmt.Errorf("create ticket metadata: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("secure ticket metadata: %w", err)
	}
	if err := json.NewEncoder(temporary).Encode(values); err != nil {
		temporary.Close()
		return fmt.Errorf("write ticket metadata: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close ticket metadata: %w", err)
	}
	if err := os.Rename(temporaryPath, filepath.Join(gitDir, ticketMetadataName)); err != nil {
		return fmt.Errorf("publish ticket metadata: %w", err)
	}
	return nil
}

func validateTicketMetadata(values map[string]string) error {
	for key := range values {
		if !urltemplate.ValidPlaceholder(key) {
			return fmt.Errorf("ticket metadata key %q is not a valid identifier", key)
		}
	}
	return nil
}
