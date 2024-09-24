package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRun(t *testing.T) {
	_, err := exec.LookPath("git")
	if err != nil {
		t.Skip("Git is not installed. Skipping test.")
	}

	testCases := []struct {
		name     string
		proj     string
		out      string
		expErr   error
		setUpGit bool
	}{
		{name: "success", proj: "./testdata/tool", out: "GO Build: SUCCESS\nGO Test: SUCCESS\nGO Fmt: SUCCESS\nGit Push: SUCCESS\n", expErr: nil, setUpGit: true},
		{name: "fail", proj: "./testdata/toolErr", out: "", expErr: &stepErr{step: "go build"}},
		{name: "failFmt", proj: "./testdata/toolFmtErr", out: "", expErr: &stepErr{step: "go fmt"}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setUpGit {
				setUpGit(t, tc.proj)
			}

			var out bytes.Buffer
			err := run(tc.proj, &out)
			if tc.expErr != nil {
				if err == nil {
					t.Errorf("Expected error: %q. Got 'nil' instead", tc.expErr)
					return
				}
				if !errors.Is(err, tc.expErr) {
					t.Errorf("Expected error: %q. Got %q instead", tc.expErr, err)
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %q", err)
			}

			if out.String() != tc.out {
				t.Errorf("Expected: %s, got: %s", tc.out, out.String())
			}
		})
	}
}

func setUpGit(t *testing.T, proj string) {
	t.Helper()
	// Check if git command is available
	gitExec, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}

	tempDir, err := os.MkdirTemp(t.TempDir(), "gocitest")
	if err != nil {
		t.Fatal(err)
	}

	projPath, err := filepath.Abs(proj)
	if err != nil {
		t.Fatal(err)
	}

	remoteUri := fmt.Sprintf("file://%s", tempDir)

	// Clean up the .git folder in the test proj path that was pass in
	t.Cleanup(func() {
		os.RemoveAll(filepath.Join(projPath, ".git"))
	})

	var gitCmdList = []struct {
		args []string
		dir  string
		env  []string
	}{
		{[]string{"init", "--bare"}, tempDir, nil},
		{[]string{"init"}, projPath, nil},
		{[]string{"remote", "add", "origin", remoteUri}, projPath, nil},
		{[]string{"add", "."}, projPath, nil},
		{[]string{"commit", "-m", "test"}, projPath, []string{"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com", "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com"}},
	}

	for _, g := range gitCmdList {
		gitCmd := exec.Command(gitExec, g.args...)
		gitCmd.Dir = g.dir

		if g.env != nil {
			gitCmd.Env = append(os.Environ(), g.env...)
		}

		if err := gitCmd.Run(); err != nil {
			t.Fatal(err)
		}
	}
}
