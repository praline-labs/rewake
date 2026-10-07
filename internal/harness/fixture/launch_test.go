//go:build rewakefixture

package fixture

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/role"
)

func TestAVersionBelowTheMinimumIsRefused(t *testing.T) {
	for version, refused := range map[string]bool{"0.9.9": true, "0.10.0": true, "1.0.0": false, "1.0.1": false, "1.10.0": false, "2.0.0": false} {
		program := versionScript(t, "#!/bin/sh\necho fixture "+version+"\n")
		_, err := New().(harness.LaunchVersionReader).ReadLaunchVersion(program, os.Environ(), t.TempDir())
		if (err != nil) != refused {
			t.Errorf("%s: refused %v: %v", version, err != nil, err)
		}
	}
}

func TestAnUnreadableVersionIsRefused(t *testing.T) {
	for name, script := range map[string]string{
		"no version": "#!/bin/sh\necho fixture\n",
		"failing":    "#!/bin/sh\necho fixture 1.0.0\nexit 3\n",
		"missing":    "",
	} {
		program := filepath.Join(t.TempDir(), "absent")
		if script != "" {
			program = versionScript(t, script)
		}
		_, err := New().(harness.LaunchVersionReader).ReadLaunchVersion(program, os.Environ(), t.TempDir())
		if err == nil || !strings.Contains(err.Error(), "could not be read") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func versionScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture")
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// marker stands for what a person's configuration or arguments hold: a
// launch's diagnostics say where something is, never what it is (L2).
const marker = "secret-marker-7f3a"

// person lays out a home with the fixture's configuration and a working
// directory, each holding the marker, and makes them the test's.
func person(t *testing.T, config string) (home, cwd string) {
	t.Helper()
	home, cwd = t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, configDir), 0o700); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		filepath.Join(home, configDir, "config"): config,
		filepath.Join(home, ".profile"):          "export TOKEN=" + marker + "\n",
		filepath.Join(cwd, "notes.txt"):          marker + "\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Chdir(cwd)
	return home, cwd
}

// launches are the argument lists a person may give, the marker in each.
var launches = [][]string{
	nil,
	{"--model", marker},
	{"--", "a prompt with " + marker},
	{"--session=" + marker},
	{"--keep", "--connect", marker},
	{"--brief", marker},
}

// L1: a launch reads the person's home and working directory and writes
// nothing to either — the mail is added on the command line and nothing else
// changes.
func TestALaunchLeavesThePersonsFilesAsTheyWere(t *testing.T) {
	home, cwd := person(t, "brief = "+marker+"\n"+marker+" = on\n")
	before := []map[string]string{digestTree(t, home), digestTree(t, cwd)}
	for _, args := range launches {
		for _, intro := range []bool{true, false} {
			_, _ = New().Launch(launchRequest(t, args, intro))
		}
	}
	after := []map[string]string{digestTree(t, home), digestTree(t, cwd)}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("a launch changed the person's files")
	}
}

// L2: every refusal and every note of a launch, with the marker in the
// configuration and in the arguments, says where and never what.
func TestALaunchsDiagnosticsNeverCarryWhatTheyPointAt(t *testing.T) {
	for _, config := range []string{
		"brief = " + marker + "\n",
		marker + " = on\nnot a setting " + marker + "\n",
	} {
		person(t, config)
		said := 0
		for _, args := range launches {
			for _, intro := range []bool{true, false} {
				plan, err := New().Launch(launchRequest(t, args, intro))
				var diagnostics []string
				if err != nil {
					diagnostics = append(diagnostics, err.Error())
				}
				diagnostics = append(diagnostics, plan.Notes...)
				for _, line := range diagnostics {
					said++
					if strings.Contains(line, marker) {
						t.Errorf("config %q, args %q: %q", config, args, line)
					}
				}
			}
		}
		if said == 0 {
			t.Fatalf("config %q: no launch said anything, so nothing was checked", config)
		}
	}
	// The version's refusal is a diagnostic of the launch too.
	program := versionScript(t, "#!/bin/sh\necho fixture "+marker+"\n")
	if _, err := New().(harness.LaunchVersionReader).ReadLaunchVersion(program, os.Environ(), t.TempDir()); err == nil || strings.Contains(err.Error(), marker) {
		t.Errorf("the version's refusal: %v", err)
	}
}

func TestACallerCannotPassTheFixturesOwnFlags(t *testing.T) {
	person(t, "")
	for _, args := range [][]string{{"--session", "x"}, {"--keep", "--connect=x"}} {
		if _, err := New().Launch(launchRequest(t, args, false)); err == nil {
			t.Errorf("%q was taken", args)
		}
	}
	if _, err := New().Launch(launchRequest(t, []string{"--", "--connect"}, false)); err != nil {
		t.Errorf("a prompt after -- was refused: %v", err)
	}
}

func launchRequest(t *testing.T, args []string, intro bool) harness.LaunchRequest {
	return harness.LaunchRequest{Name: "api", Dir: t.TempDir(), Room: "default", Args: args, Intro: intro, Epoch: "e1", Role: role.General}
}

// digestTree maps every path under root to its mode and a digest of its
// contents.
func digestTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		sum := ""
		if entry.Type().IsRegular() {
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			digest := sha256.Sum256(raw)
			sum = hex.EncodeToString(digest[:])
		}
		out[strings.TrimPrefix(path, root)] = info.Mode().String() + " " + sum
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
