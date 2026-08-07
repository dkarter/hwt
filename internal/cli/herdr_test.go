package cli

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestNormalizeBranchInput(t *testing.T) {
	if got := normalizeBranchInput("  feature/my new\tthing  "); got != "feature/my-new-thing" {
		t.Fatalf("normalizeBranchInput() = %q", got)
	}
}

func TestVimInputChangesModeAndCursorShape(t *testing.T) {
	input := newVimInput("")
	if input.mode != modeInsert || input.cursor(0).Shape != tea.CursorBar {
		t.Fatal("input should start in insert mode with a bar cursor")
	}
	input.update(specialKey(tea.KeyEscape))
	if input.mode != modeNormal || input.cursor(0).Shape != tea.CursorBlock {
		t.Fatal("escape should enter normal mode with a block cursor")
	}
	input.update(textKey("i"))
	if input.mode != modeInsert || input.cursor(0).Shape != tea.CursorBar {
		t.Fatal("i should return to insert mode with a bar cursor")
	}
}

func TestBranchPickerFuzzyRanking(t *testing.T) {
	model := newBranchPickerModel([]string{"feature/other", "main", "feature/plugin-e2e"})
	model.input.setValue("fpe")
	model.refilter()
	if len(model.matches) != 2 || model.matches[0].value != "feature/plugin-e2e" || !model.matches[1].custom {
		t.Fatalf("unexpected fuzzy matches: %#v", model.matches)
	}
}

func TestConfirmUsesHorizontalNavigationAndSafeDefault(t *testing.T) {
	model := confirmModel{}
	updated, _ := model.Update(specialKey(tea.KeyEnter))
	if updated.(confirmModel).result {
		t.Fatal("confirmation should default to no")
	}
	updated, _ = model.Update(textKey("h"))
	updated, _ = updated.(confirmModel).Update(specialKey(tea.KeyEnter))
	if !updated.(confirmModel).result {
		t.Fatal("h then enter should select yes")
	}
}

func TestVimInputUsesCellWidthAndNormalModeBoundaries(t *testing.T) {
	input := newVimInput("")
	input.setValue("界a")
	input.setCursor(2)
	if got := input.cursor(0).Position.X; got != 5 {
		t.Fatalf("cursor X = %d, want 5", got)
	}
	input.setMode(modeNormal)
	input.update(textKey("l"))
	if input.position != 1 {
		t.Fatalf("normal-mode l moved past final character: %d", input.position)
	}
	input.update(textKey("$"))
	if input.position != 1 {
		t.Fatalf("normal-mode $ moved past final character: %d", input.position)
	}
}

func TestVimInputScrollsCursorWithinWidth(t *testing.T) {
	input := newVimInput("")
	input.setWidth(4)
	input.setValue("abcdefgh")
	input.setCursor(8)
	if got := input.cursor(0).Position.X; got > 9 {
		t.Fatalf("cursor X = %d, outside input viewport", got)
	}
}

func TestVimWordMotions(t *testing.T) {
	tests := []struct {
		name     string
		position int
		motion   string
		want     int
	}{
		{name: "e", position: 0, motion: "e", want: 2},
		{name: "w", position: 0, motion: "w", want: 4},
		{name: "b", position: 10, motion: "b", want: 8},
		{name: "B", position: 10, motion: "B", want: 4},
		{name: "E", position: 4, motion: "E", want: 12},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := normalInput("one two-three FOUR", test.position)
			input.update(textKey(test.motion))
			if input.position != test.want {
				t.Fatalf("position = %d, want %d", input.position, test.want)
			}
		})
	}
}

func TestVimInsertCommandsAndSubstitute(t *testing.T) {
	tests := []struct {
		name     string
		position int
		keys     []string
		want     string
	}{
		{name: "A", position: 0, keys: []string{"A", "X"}, want: "oneX"},
		{name: "I", position: 4, keys: []string{"I", "X"}, want: "  Xone"},
		{name: "substitute", position: 1, keys: []string{"s", "X"}, want: "oXe"},
		{name: "substitute final character", position: 2, keys: []string{"s", "X"}, want: "onX"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := "one"
			if test.name == "I" {
				value = "  one"
			}
			input := normalInput(value, test.position)
			for _, key := range test.keys {
				input.update(textKey(key))
			}
			if got := input.valueString(); got != test.want {
				t.Fatalf("value = %q, want %q", got, test.want)
			}
			if input.mode != modeInsert {
				t.Fatal("command should finish in insert mode")
			}
		})
	}
}

func TestEscapeReturnsCursorToInsertedCharacter(t *testing.T) {
	input := normalInput("abcd", 2)
	input.update(textKey("i"))
	input.update(textKey("X"))
	input.update(specialKey(tea.KeyEscape))
	if input.valueString() != "abXcd" || input.position != 2 {
		t.Fatalf("value = %q, position = %d", input.valueString(), input.position)
	}
}

func TestOperatorPendingConsumesApplicationKeys(t *testing.T) {
	model := newBranchPickerModel([]string{"main", "next"})
	model.input.setValue("one two")
	model.input.setCursor(0)
	model.input.setMode(modeNormal)
	updated, _ := model.Update(textKey("d"))
	updated, _ = updated.(branchPickerModel).Update(textKey("j"))
	result := updated.(branchPickerModel)
	if result.selected != 0 || result.input.deleting {
		t.Fatalf("operator key leaked to picker: selected=%d deleting=%v", result.selected, result.input.deleting)
	}
	if result.input.valueString() != "one two" {
		t.Fatalf("invalid delete motion changed input: %q", result.input.valueString())
	}
}

func TestVimDeleteRespectsMotions(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		position int
		motion   string
		want     string
	}{
		{name: "dw", value: "one two-three FOUR", position: 0, motion: "w", want: "two-three FOUR"},
		{name: "de", value: "one two-three FOUR", position: 0, motion: "e", want: " two-three FOUR"},
		{name: "db", value: "one two-three FOUR", position: 10, motion: "b", want: "one two-ree FOUR"},
		{name: "dB", value: "one two-three FOUR", position: 10, motion: "B", want: "one ree FOUR"},
		{name: "dE", value: "one two-three FOUR", position: 4, motion: "E", want: "one  FOUR"},
		{name: "dh", value: "one", position: 1, motion: "h", want: "ne"},
		{name: "dl", value: "one", position: 1, motion: "l", want: "oe"},
		{name: "d0", value: "one", position: 2, motion: "0", want: "e"},
		{name: "d$", value: "one", position: 1, motion: "$", want: "o"},
		{name: "dd", value: "one", position: 1, motion: "d", want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := normalInput(test.value, test.position)
			input.update(textKey("d"))
			if !input.deleting {
				t.Fatal("d should enter operator-pending mode")
			}
			input.update(textKey(test.motion))
			if got := input.valueString(); got != test.want {
				t.Fatalf("value = %q, want %q", got, test.want)
			}
			if input.deleting {
				t.Fatal("delete motion should leave operator-pending mode")
			}
		})
	}
}

func TestCurrentPluginContextPrefersForwardedActionContext(t *testing.T) {
	t.Setenv("HWT_HERDR_PLUGIN_CONTEXT_JSON", `{"workspace_id":"original","focused_pane_cwd":"/original"}`)
	t.Setenv("HERDR_PLUGIN_CONTEXT_JSON", `{"workspace_id":"popup","focused_pane_cwd":"/popup"}`)

	context, err := currentPluginContext()
	if err != nil {
		t.Fatal(err)
	}
	if context.WorkspaceID != "original" || context.FocusedPaneCWD != "/original" {
		t.Fatalf("unexpected context: %#v", context)
	}
}

func TestNewUsesHerdrBinPath(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "/custom/herdr")
	command := New("test")
	if got := command.PersistentFlags().Lookup("herdr-bin").DefValue; got != "/custom/herdr" {
		t.Fatalf("--herdr-bin default = %q", got)
	}
}

func textKey(value string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Text: value, Code: []rune(value)[0]}
}

func specialKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func normalInput(value string, position int) vimInput {
	input := newVimInput("")
	input.setValue(value)
	input.setCursor(position)
	input.setMode(modeNormal)
	return input
}
