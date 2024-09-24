package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

type executer interface {
	execute() (string, error)
}

func main() {
	proj := flag.String("p", "", "Project directory")
	flag.Parse()

	if err := run(*proj, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(proj string, out io.Writer) error {
	if proj == "" {
		return fmt.Errorf("Project directory is required: %w", ErrValidation)
	}

	pipeline := make([]executer, 4)
	pipeline[0] = newStep("go build", "go", proj, "GO Build: SUCCESS", []string{"build", ".", "errors"})
	pipeline[1] = newStep("go test", "go", proj, "GO Test: SUCCESS", []string{"test", "-v", "errors"})
	pipeline[2] = newExeptionStep("go fmt", "gofmt", proj, "GO Fmt: SUCCESS", []string{"-l", "."})
	pipeline[3] = newTimeoutStep("go push", "git", proj, "Git Push: SUCCESS", []string{"push", "origin", "main"}, 10*time.Second)

	for _, s := range pipeline {
		msg, err := s.execute()
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out, msg); err != nil {
			return err
		}
	}

	return nil
}
