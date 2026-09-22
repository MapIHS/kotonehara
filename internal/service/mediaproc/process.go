// Package mediaproc bounds expensive local media processes across bot features.
package mediaproc

import (
	"context"
	"os/exec"
	"time"
)

// Two processes are sufficient for a small bot and avoid CPU spikes.
var slots = make(chan struct{}, 2)

type Cmd struct {
	*exec.Cmd
	ctx    context.Context
	cancel context.CancelFunc
}

func CommandContext(ctx context.Context, name string, args ...string) *Cmd {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 5 * time.Second
	return &Cmd{Cmd: cmd, ctx: ctx, cancel: cancel}
}

func Command(name string, args ...string) *Cmd {
	return CommandContext(context.Background(), name, args...)
}

func LookPath(name string) (string, error) { return exec.LookPath(name) }

func (c *Cmd) acquire() error {
	if err := c.ctx.Err(); err != nil {
		return err
	}
	select {
	case slots <- struct{}{}:
		if err := c.ctx.Err(); err != nil {
			<-slots
			return err
		}
		return nil
	case <-c.ctx.Done():
		return c.ctx.Err()
	}
}
func (c *Cmd) Run() error {
	defer c.cancel()
	if err := c.acquire(); err != nil {
		return err
	}
	defer func() { <-slots }()
	return c.Cmd.Run()
}
func (c *Cmd) Output() ([]byte, error) {
	defer c.cancel()
	if err := c.acquire(); err != nil {
		return nil, err
	}
	defer func() { <-slots }()
	return c.Cmd.Output()
}
func (c *Cmd) CombinedOutput() ([]byte, error) {
	defer c.cancel()
	if err := c.acquire(); err != nil {
		return nil, err
	}
	defer func() { <-slots }()
	return c.Cmd.CombinedOutput()
}
