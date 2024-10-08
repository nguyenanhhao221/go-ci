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
	"reflect"
	"syscall"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestRun(t *testing.T) {
	testCases := []struct {
		name     string
		proj     string
		gBranch  string
		yamlFile string
		out      string
		expErr   error
		setUpGit bool
		mockCmd  func(ctx context.Context, name string, arg ...string) *exec.Cmd
	}{
		{name: "success", proj: "./testdata/tool", out: "GO Build: SUCCESS\nGO Lint: SUCCESS\nGO Test: SUCCESS\nGO Fmt: SUCCESS\nGit Push: SUCCESS\n", expErr: nil, setUpGit: true, mockCmd: nil, yamlFile: "pipeline.yaml"},
		{name: "successMock", proj: "./testdata/tool", out: "GO Build: SUCCESS\nGO Lint: SUCCESS\nGO Test: SUCCESS\nGO Fmt: SUCCESS\nGit Push: SUCCESS\n", expErr: nil, setUpGit: false, mockCmd: mockCmdContext},
		{name: "fail", proj: "./testdata/toolErr", out: "", expErr: &stepErr{step: "go-build"}, yamlFile: "../pipeline.yaml"},
		{name: "failFmt", proj: "./testdata/toolFmtErr", out: "", expErr: &stepErr{step: "go-fmt"}},
		{name: "failLint", proj: "./testdata/toolLintErr", out: "", expErr: &stepErr{step: "go-lint"}, setUpGit: false, mockCmd: nil},
		{name: "failGitBranch", proj: "./testdata/tool", out: "", expErr: &stepErr{step: "git-push"}, setUpGit: true, mockCmd: nil, gBranch: "foo", yamlFile: "pipeline.yaml"},
		{name: "failTimeout", proj: "./testdata/tool", out: "", expErr: context.DeadlineExceeded, setUpGit: false, mockCmd: mockCmdTimeout},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// This is to avoid case when the command is mock for other test
			// Because command is define globally and base on the order of our test tables, if test 1 make command into a mock, it will persist to below test unless cleanup properly
			originalCommand := command // Save the original command

			t.Cleanup(func() {
				command = originalCommand
			})

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

			var gBranch = "main"
			var yamlFile = "../pipeline.yaml"
			if tc.gBranch != "" {
				gBranch = tc.gBranch
			}

			if tc.yamlFile != "" {
				yamlFile = tc.yamlFile
			}

			err := run(tc.proj, gBranch, yamlFile, &out)
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
				errCh <- run(tc.proj, "main", "pipeline.yaml", io.Discard)
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

func TestGetPipelingFromFile(t *testing.T) {
	// Define the expected pipeline structure (should match what's in pipeline.yaml)
	expected := PipeLine{
		{
			"go-build": Job{
				Command:    "go",
				Args:       []string{"build", "."},
				SuccessMsg: "GO Build: SUCCESS",
				Timeout:    0,
			},
		},
		{
			"go-lint": Job{
				Command:    "golangci-lint",
				Args:       []string{"run"},
				SuccessMsg: "GO Lint: SUCCESS",
				Timeout:    0,
			},
		},
		{
			"go-test": Job{
				Command:    "go",
				Args:       []string{"test", "-v"},
				SuccessMsg: "GO Test: SUCCESS",
				Timeout:    0,
			},
		},
		{
			"go-fmt": Job{
				Command:    "gofmt",
				Args:       []string{"-l", "."},
				SuccessMsg: "GO Fmt: SUCCESS",
				Timeout:    0,
			},
		},
		{
			"git-push": Job{
				Command:    "git",
				Args:       []string{"push", "origin", "main"},
				SuccessMsg: "Git Push: SUCCESS",
				Timeout:    10 * time.Second, // Adjust the timeout based on your YAML content
			},
		},
	}
	out, err := getPipelineFromFile("pipeline.yaml", "./testdata/tool")
	if err != nil {
		t.Errorf("Unexpected error %q", err)
	}

	if !reflect.DeepEqual(out, expected) {
		t.Error(cmp.Diff(out, expected))
	}
}
