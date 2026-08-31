package ui

import (
	"reflect"
	"testing"
)

func TestEditorCommandAcceptsArguments(t *testing.T) {
	path := "/tmp/k10s service.yaml"
	cmd, err := editorCommand("code --wait --reuse-window", path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"code", "--wait", "--reuse-window", path}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("editor args = %#v, want %#v", cmd.Args, want)
	}
}

func TestEditorCommandAcceptsQuotedExecutablePath(t *testing.T) {
	path := "/tmp/service.yaml"
	cmd, err := editorCommand(`"/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code" --wait`, path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/Applications/Visual Studio Code.app/Contents/Resources/app/bin/code",
		"--wait",
		path,
	}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("editor args = %#v, want %#v", cmd.Args, want)
	}
}

func TestEditorCommandDefaultsToVi(t *testing.T) {
	cmd, err := editorCommand("  ", "/tmp/service.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if got := cmd.Args[0]; got != "vi" {
		t.Fatalf("default editor = %q, want vi", got)
	}
}

func TestEditorCommandRejectsMalformedQuotes(t *testing.T) {
	if _, err := editorCommand(`code "--wait`, "/tmp/service.yaml"); err == nil {
		t.Fatal("malformed $EDITOR returned no error")
	}
}
