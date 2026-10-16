package grade

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// runFile runs one test command in its own process group with a timeout. On timeout the whole group is
// killed; after a normal exit, anything the tests left running in the group is killed too. A non-nil
// error means the command could not be run at all.
func runFile(dir string, timeout time.Duration, name string, args ...string) (exitCode int, timedOut bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	err = cmd.Run()
	if cmd.Process != nil {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	if ctx.Err() == context.DeadlineExceeded {
		return -1, true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), false, nil
	}
	return 0, false, err
}
