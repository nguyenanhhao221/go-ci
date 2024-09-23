package main

import (
	"bytes"
	"fmt"
	"os/exec"
)

type exeptionStep struct {
	step
}

func (s exeptionStep) execute() (string, error) {
	var out bytes.Buffer

	cmd := exec.Command(s.exe, s.args...)
	cmd.Stdout = &out
	cmd.Dir = s.proj

	if err := cmd.Run(); err != nil {
		return "", &stepErr{
			step:  s.name,
			msg:   "failed to execute",
			cause: err,
		}
	}

	if out.Len() > 0 {
		return "", &stepErr{
			step:  s.name,
			msg:   fmt.Sprintf("invalid format: %s", out.String()),
			cause: nil,
		}
	}
	return s.message, nil
}

func newExeptionStep(name, exe, proj, message string, args []string) exeptionStep {
	s := exeptionStep{}
	s.step = newStep(name, exe, proj, message, args)
	return s
}
