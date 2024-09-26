package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

type executer interface {
	execute() (string, error)
}

type Job struct {
	Command    string        `yaml:"command"`
	Args       []string      `yaml:"args"`
	SuccessMsg string        `yaml:"success_message"`
	Timeout    time.Duration `yaml:"timeout,omitempty"`
}

type PipeLine []map[string]Job

func main() {
	proj := flag.String("p", "", "Project directory")
	gitBranch := flag.String("branch", "main", "Git branch to push")
	yamlFile := flag.String("f", "pipeline.yaml", "YAML file defining the pipeline")
	flag.Parse()

	if err := run(*proj, *gitBranch, *yamlFile, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(proj, gitBranch, yamlFile string, out io.Writer) error {
	if proj == "" {
		return fmt.Errorf("Project directory is required: %w", ErrValidation)
	}

	pipeLine, err := getPipelineFromFile(yamlFile, proj)
	if err != nil {
		return err
	}

	steps := make([]executer, 0)
	for _, jobMap := range pipeLine {
		for jobName, job := range jobMap {
			switch job.Command {
			case "gofmt":
				steps = append(steps, newExeptionStep(jobName, job.Command, proj, job.SuccessMsg, job.Args))
			case "git":
				handleGitCommand(&job, gitBranch) // Extracted logic for handling git commands
				steps = append(steps, newTimeoutStep(jobName, job.Command, proj, job.SuccessMsg, job.Args, job.Timeout))
			default:
				if job.Timeout > 0 {
					steps = append(steps, newTimeoutStep(jobName, job.Command, proj, job.SuccessMsg, job.Args, job.Timeout))
				} else {
					steps = append(steps, newStep(jobName, job.Command, proj, job.SuccessMsg, job.Args))
				}
			}
		}
	}

	// Use signal channel to "listen" to terminal signal if they were sent to the application
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	errCh := make(chan error)
	doneCh := make(chan struct{})

	go func() {
		for _, s := range steps {
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

func getPipelineFromFile(yamlPath, proj string) (PipeLine, error) {
	yamlAbsPath := filepath.Join(proj, yamlPath)
	var root yaml.Node

	f, err := os.ReadFile(yamlAbsPath)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(f, &root); err != nil {
		return nil, err
	}

	var pipeLine PipeLine
	// Iterate over the document to preserve the order of jobs
	for i := 0; i < len(root.Content[0].Content); i += 2 {
		jobNameNode := root.Content[0].Content[i]
		jobDataNode := root.Content[0].Content[i+1]

		var job Job
		err := jobDataNode.Decode(&job)
		if err != nil {
			return nil, err
		}

		// Create a map for the job with the job name as the key
		jobMap := map[string]Job{
			jobNameNode.Value: job,
		}
		pipeLine = append(pipeLine, jobMap)
	}
	return pipeLine, nil
}

func handleGitCommand(job *Job, gitBranch string) {
	if len(job.Args) >= 3 && job.Args[0] == "push" {
		if gitBranch != "" {
			// When user specify -branch flag, it has higher priority than in the yaml file
			job.Args[2] = gitBranch
		}
	}
}
