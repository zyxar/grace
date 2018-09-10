package fork

import (
	"io"
	"os/exec"
)

// Option describes stdout, stdin, stderr and env settings
// for spawning a new process
type Option struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Env    []string
}

// Daemonize starts a new process specified by bin;
// it returns an error when encounter any failures;
// upon success, it releases the child process and return nil;
// the stdout, stderr of the child process are redirected intended;
func Daemonize(bin string, opt *Option, args ...string) (int, error) {
	cmd, err := spawn(bin, opt, args...)
	if err != nil {
		return -1, err
	}
	pid := cmd.Process.Pid
	return pid, cmd.Process.Release()
}

// Exec starts a new process specified by bin and waits for it to complete;
// it returns an error when encounter any failures;
// the stdout, stderr of the child process are redirected intended;
func Exec(bin string, opt *Option, args ...string) error {
	cmd, err := spawn(bin, opt, args...)
	if err != nil {
		return err
	}
	return cmd.Wait()
}

func spawn(bin string, opt *Option, args ...string) (*exec.Cmd, error) {
	cmd := exec.Command(bin, args...)
	if opt != nil {
		if opt.Stdin != nil {
			cmd.Stdin = opt.Stdin
		}
		if opt.Stdout != nil {
			cmd.Stdout = opt.Stdout
		}
		if opt.Stderr != nil {
			cmd.Stderr = opt.Stderr
		}
		if opt.Env != nil {
			cmd.Env = opt.Env
		}
	}
	return cmd, cmd.Start()
}
