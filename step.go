package main

import "os/exec"

type step struct {
	name    string
	exe     string
	proj    string
	message string
	args    []string
}

func (s step) execute() (string, error) {
	cmd := exec.Command(s.exe, s.args...)
	cmd.Dir = s.proj

	if err := cmd.Run(); err != nil {
		return "", &stepErr{
			step:  s.name,
			msg:   "failed to execute",
			cause: err,
		}
	}

	return s.message, nil
}

func newStep(name, exe, proj, message string, args []string) step {
	return step{
		name:    name,
		exe:     exe,
		proj:    proj,
		message: message,
		args:    args,
	}
}
