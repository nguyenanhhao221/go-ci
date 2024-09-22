package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
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

	// In the first step we will run go build program. However, if we just run "go build .", go will output an executable file,  then we will need to cleanup this file after
	// However, when go build multiple packages, it doesn't output any files. so we can use the "errors" standard package in the build command
	// This bascially run "go build . errors"
	args := []string{"build", ".", "errors"}
	cmd := exec.Command("go", args...)
	cmd.Dir = proj

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("'go build' failed: %s", err)
	}
	_, err := fmt.Fprintln(out, "GO Build: SUCCESS")
	return err
}
