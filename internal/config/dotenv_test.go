package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeEnv(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadDotEnvSyntax(t *testing.T) {
	p := writeEnv(t, `# a comment
   # indented comment

A=1
export B=two
  C  =  spaced value  
D="double quoted"
E='single quoted'
F=a=b=c
G=
H="mismatched'
I=${A}
J=x # not a comment
`)
	got, err := ReadDotEnv(p)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"A": "1", "B": "two", "C": "spaced value", "D": "double quoted", "E": "single quoted",
		"F": "a=b=c", "G": "", "H": `"mismatched'`, "I": "${A}", "J": "x # not a comment",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestReadDotEnvMissingFile(t *testing.T) {
	got, err := ReadDotEnv(filepath.Join(t.TempDir(), "nope"))
	if got != nil || err != nil {
		t.Errorf("got %v, %v; want nil, nil", got, err)
	}
}

func TestReadDotEnvBadLine(t *testing.T) {
	p := writeEnv(t, "A=1\n\nthis is bad\n")
	_, err := ReadDotEnv(p)
	if err == nil || !strings.Contains(err.Error(), ":3:") || !strings.Contains(err.Error(), p) {
		t.Errorf("error = %v; want file and line 3", err)
	}
}
