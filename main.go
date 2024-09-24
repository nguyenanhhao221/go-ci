package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type executer interface {
	execute() (string, error)
}

func main() {
	proj := flag.String("p", "", "Project directory")
	flag.Parse()

	gitBranches, err := getAvailableGitBranches()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting git branches: %s\n", err)
		os.Exit(1)
	}

	fmt.Println("Select a branch")
	for i, branch := range gitBranches {
		fmt.Printf("[%d] %s\n", i+1, branch)
	}

	fmt.Println("Input a number to select the branch: ")
	r := bufio.NewReader(os.Stdin)
	input, err := r.ReadString('\n')
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error when getting input: %s\n", err)
		os.Exit(1)
	}
	input = strings.TrimSpace(input)

	// Convert user input to the branch
	selectIndex := 0
	if _, err := fmt.Sscan(input, &selectIndex); err != nil || selectIndex < 1 || selectIndex > len(gitBranches) {
		fmt.Fprintf(os.Stderr, "Invalid branch selection: %s\n", err)
		os.Exit(1)
	}

	selectedBranch := gitBranches[selectIndex]

	if err := run(*proj, os.Stdout, selectedBranch); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(proj string, out io.Writer, gitBranch string) error {
	if proj == "" {
		return fmt.Errorf("Project directory is required: %w", ErrValidation)
	}

	pipeline := make([]executer, 5)
	pipeline[0] = newStep("go build", "go", proj, "GO Build: SUCCESS", []string{"build", ".", "errors"})
	pipeline[1] = newStep("go lint", "golangci-lint", proj, "GO Lint: SUCCESS", []string{"run"})
	pipeline[2] = newStep("go test", "go", proj, "GO Test: SUCCESS", []string{"test", "-v", "errors"})
	pipeline[3] = newExeptionStep("go fmt", "gofmt", proj, "GO Fmt: SUCCESS", []string{"-l", "."})
	pipeline[4] = newTimeoutStep("go push", "git", proj, "Git Push: SUCCESS", []string{"push", "origin", gitBranch}, 10*time.Second)

	// Use signal channel to "listen" to terminal signal if they were sent to the application
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	errCh := make(chan error)
	doneCh := make(chan struct{})

	go func() {
		for _, s := range pipeline {
			msg, err := s.execute()
			if err != nil {
				errCh <- err
				return
			}
			if _, err := fmt.Fprintln(out, msg); err != nil {
				errCh <- err
				return
			}
		}
		close(doneCh)
	}()

	for {
		select {
		case rec := <-sigCh:
			signal.Stop(sigCh)
			return fmt.Errorf("%s: Exiting: %w", rec, ErrSignal)
		case err := <-errCh:
			return err
		case <-doneCh:
			return nil
		}
	}
}

func getAvailableGitBranches() ([]string, error) {
	cmd := exec.Command("git", "branch")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	branches := strings.Split(string(output), "\n")
	var cleanedBranches []string
	for _, branch := range branches {
		cleanedBranch := strings.TrimSpace(branch)
		if cleanedBranch != "" {
			cleanedBranch = strings.TrimPrefix(cleanedBranch, "*")
			cleanedBranches = append(cleanedBranches, cleanedBranch)
		}
	}

	return cleanedBranches, nil
}
