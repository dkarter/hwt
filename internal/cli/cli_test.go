package cli

import (
	"bytes"
	"testing"

	"github.com/dkarter/hwt/skills"
)

func TestSkillCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []byte
	}{
		{name: "usage", args: []string{"skill"}, want: skills.Usage},
		{name: "config reference", args: []string{"skill", "config"}, want: skills.ProjectConfig},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := New("test")
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetArgs(test.args)

			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(output.Bytes(), test.want) {
				t.Fatalf("unexpected output for hwt %v", test.args)
			}
		})
	}
}
