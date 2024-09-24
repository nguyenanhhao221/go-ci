package main

import (
	"context"
	"os/exec"
	"time"
)

type timeoutStep struct {
	step
	timeout time.Duration
}

func (s timeoutStep) execute() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)

	cmd := exec.CommandContext(ctx, s.exe, s.args...)
	defer cancel()

	cmd.Dir = s.proj

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", &stepErr{
				step:  s.name,
				msg:   "failed time out",
				cause: context.DeadlineExceeded,
			}
		}

		return "", &stepErr{
			step:  s.name,
			msg:   "failed to execute",
			cause: err,
		}
	}

	return s.message, nil

}

func newTimeoutStep(name, exe, proj, message string, args []string, timeout time.Duration) timeoutStep {
	s := timeoutStep{}
	s.step = newStep(name, exe, proj, message, args)
	s.timeout = timeout

	// Set default time out to 30s
	if s.timeout == 0 {
		s.timeout = 30 * time.Second
	}

	return s
}
