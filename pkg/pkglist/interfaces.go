package pkglist

import "os/exec"

// Commander executes commands and returns their output
type Commander interface {
	Command(name string, args ...string) Command
}

// Command represents a runnable command
type Command interface {
	SetDir(dir string)
	Output() ([]byte, error)
}

// RealCommander implements Commander using os/exec
type RealCommander struct{}

func (c *RealCommander) Command(name string, args ...string) Command {
	return &RealCommand{
		cmd: exec.Command(name, args...),
	}
}

// RealCommand wraps exec.Cmd
type RealCommand struct {
	cmd *exec.Cmd
}

func (c *RealCommand) SetDir(dir string) {
	c.cmd.Dir = dir
}

func (c *RealCommand) Output() ([]byte, error) {
	return c.cmd.Output()
}
