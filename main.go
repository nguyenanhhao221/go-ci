package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

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

	//TODO: extend this pipeline length
	pipeline := make([]step, 2)
	pipeline[0] = newStep("go build", "go", proj, "GO Build: SUCCESS", []string{"build", ".", "errors"})
	pipeline[1] = newStep("go test", "go", proj, "GO Test: SUCCESS", []string{"test", "-v", "errors"})

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
