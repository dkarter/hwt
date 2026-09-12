package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/dkarter/hwt/internal/speclink"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || (args[0] != "validate" && args[0] != "run") {
		return fmt.Errorf("usage: spec-links <validate|run> [options]")
	}
	command := args[0]
	flags := flag.NewFlagSet("spec-links "+command, flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	specRoot := flags.String("spec-root", "openspec/specs", "OpenSpec specs directory")
	e2eRoot := flags.String("e2e-root", "e2e", "E2E Go package directory")
	scenario := flags.String("scenario", "", "select one scenario ID")
	capability := flags.String("capability", "", "select one capability directory")
	spec := flags.String("spec", "", "select one spec file path")
	list := flags.Bool("list", false, "list selected tests without running them")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}

	index, err := speclink.Load(*specRoot, *e2eRoot)
	if err != nil {
		return err
	}
	if command == "validate" {
		fmt.Printf("validated %d scenarios and %d E2E tests\n", len(index.Scenarios), len(index.Tests))
		return nil
	}
	tests, err := index.Select(*scenario, *capability, *spec)
	if err != nil {
		return err
	}
	for _, test := range tests {
		fmt.Printf("%s\t%s\n", strings.Join(test.IDs, ","), test.Name)
	}
	if *list {
		return nil
	}
	cmd := exec.Command("go", "test", "./e2e", "-run", speclink.TestRegex(tests))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
