package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestRun(t *testing.T) {
	testCases := []struct {
		name     string
		proj     string
		out      string
		expErr   error
		setUpGit bool
		mockCmd  func(ctx context.Context, name string, arg ...string) *exec.Cmd
	}{
		{name: "success", proj: "./testdata/tool", out: "GO Build: SUCCESS\nGO Lint: SUCCESS\nGO Test: SUCCESS\nGO Fmt: SUCCESS\nGit Push: SUCCESS\n", expErr: nil, setUpGit: true, mockCmd: nil},
		{name: "successMock", proj: "./testdata/tool", out: "GO Build: SUCCESS\nGO Lint: SUCCESS\nGO Test: SUCCESS\nGO Fmt: SUCCESS\nGit Push: SUCCESS\n", expErr: nil, setUpGit: false, mockCmd: mockCmdContext},
		{name: "fail", proj: "./testdata/toolErr", out: "", expErr: &stepErr{step: "go build"}},
		{name: "failFmt", proj: "./testdata/toolFmtErr", out: "", expErr: &stepErr{step: "go fmt"}},
		{name: "failLint", proj: "./testdata/toolLintErr", out: "", expErr: &stepErr{step: "go lint"}, setUpGit: false, mockCmd: nil},
		{name: "failTimeout", proj: "./testdata/tool", out: "", expErr: context.DeadlineExceeded, setUpGit: false, mockCmd: mockCmdTimeout},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setUpGit {
				_, err := exec.LookPath("git")
				if err != nil {
					t.Skip("Git is not installed. Skipping test.")
				}
				setUpGit(t, tc.proj)
			}

			if tc.mockCmd != nil {
				command = tc.mockCmd
			}

			var out bytes.Buffer
			err := run(tc.proj, &out, "main")
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

func TestRunKillSignal(t *testing.T) {
	testCases := []struct {
		name   string
		proj   string
		sig    syscall.Signal
		expErr error
	}{
		{name: "SIGINT", proj: "./testdata/tool", sig: syscall.SIGINT, expErr: ErrSignal},
		{name: "SIGTERM", proj: "./testdata/tool", sig: syscall.SIGTERM, expErr: ErrSignal},
		{name: "SIGQUIT", proj: "./testdata/tool", sig: syscall.SIGQUIT, expErr: nil},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			command = mockCmdTimeout
			errCh := make(chan error)
			expSigCh := make(chan os.Signal, 1)
			ignSigCh := make(chan os.Signal, 1)

			signal.Notify(ignSigCh, syscall.SIGQUIT)
			defer signal.Stop(ignSigCh)

			signal.Notify(expSigCh, tc.sig)
			defer signal.Stop(expSigCh)

			go func() {
				errCh <- run(tc.proj, io.Discard, "main")
			}()

			go func() {
				time.Sleep(2 * time.Second)
				err := syscall.Kill(syscall.Getpid(), tc.sig)
				if err != nil {
					t.Errorf("fail to send signal %s", err)
				}
			}()

			select {
			case err := <-errCh:
				if err == nil {
					t.Errorf("Expected error got nil instead")
					return
				}
				if !errors.Is(err, tc.expErr) {
					t.Errorf("Expect error: %q, got %q", tc.expErr, err)
				}
				select {
				case rec := <-expSigCh:
					if rec != tc.sig {
						t.Errorf("Expect signal %q, got %q", tc.sig, rec)
					}

				default:
					t.Errorf("Signal is not received")
				}
			case <-ignSigCh:
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

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	if os.Getenv("GO_HELPER_TIMEOUT") == "1" {
		time.Sleep(15 * time.Second)
	}

	if os.Args[2] == "git" {
		fmt.Println("Everything is up-to-date")
		os.Exit(0)
	}
	os.Exit(1)
}

func mockCmdContext(ctx context.Context, exe string, arg ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess"}
	cs = append(cs, exe)
	cs = append(cs, arg...)

	cmd := exec.CommandContext(ctx, os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

func mockCmdTimeout(ctx context.Context, exe string, arg ...string) *exec.Cmd {
	cmd := mockCmdContext(ctx, exe, arg...)
	cmd.Env = append(cmd.Env, "GO_HELPER_TIMEOUT=1")
	return cmd
}
