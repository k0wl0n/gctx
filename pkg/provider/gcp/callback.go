package gcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// CallbackPort is where gcloud's browser login listens for the OAuth redirect.
const CallbackPort = 8085

// PortOwner is the process listening on the callback port. Only the command
// name and pid are kept, never its arguments or environment.
type PortOwner struct {
	Command string
	PID     int
}

// PortReport is what CallbackPreflight.Check found.
type PortReport struct {
	Busy      bool // the port was in use when checked
	StillBusy bool // still in use after the wait
	Waited    time.Duration
	Owner     PortOwner
}

// CallbackPreflight checks that the login callback port is free before a
// browser login step. gcloud treats the first request on the port as the
// redirect, so a port still held by another process makes the step fail.
type CallbackPreflight struct {
	Port  int
	Wait  time.Duration // how long to wait for the port to be released
	Poll  time.Duration
	Owner func(ctx context.Context, port int) (PortOwner, error) // nil: lsof
}

// DefaultCallbackPreflight checks the real gcloud callback port.
func DefaultCallbackPreflight() *CallbackPreflight {
	return &CallbackPreflight{Port: CallbackPort, Wait: 5 * time.Second, Poll: 100 * time.Millisecond}
}

// Check waits for the port to be released and, when it stays busy, writes one
// warning line to w. gcloud still runs either way.
func (c *CallbackPreflight) Check(ctx context.Context, w io.Writer) PortReport {
	var report PortReport
	if !c.busy() {
		return report
	}
	report.Busy = true
	start := time.Now()
	poll := c.Poll
	if poll <= 0 {
		poll = 100 * time.Millisecond
	}
	timer := time.NewTimer(c.Wait)
	defer timer.Stop()
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for report.StillBusy = true; report.StillBusy; {
		select {
		case <-ctx.Done():
			report.Waited = time.Since(start)
			return report
		case <-timer.C:
			report.Waited = time.Since(start)
			report.Owner, _ = c.owner(ctx)
			c.warn(w, report.Owner)
			return report
		case <-ticker.C:
			report.StillBusy = c.busy()
		}
	}
	report.Waited = time.Since(start)
	return report
}

func (c *CallbackPreflight) warn(w io.Writer, owner PortOwner) {
	if w == nil {
		return
	}
	user := ""
	if owner.Command != "" {
		user = fmt.Sprintf(" by %s (pid %d)", owner.Command, owner.PID)
	}
	fmt.Fprintf(w, "port %d is in use%s; gcloud's login callback needs it — close it or retry\n", c.Port, user)
}

// busy reports whether the port cannot be bound on IPv4 or IPv6 loopback.
// Only "address in use" counts: a host without IPv6 is not busy.
func (c *CallbackPreflight) busy() bool {
	for _, addr := range []string{"127.0.0.1", "::1"} {
		l, err := net.Listen("tcp", net.JoinHostPort(addr, strconv.Itoa(c.Port)))
		if err == nil {
			l.Close()
			continue
		}
		if errors.Is(err, syscall.EADDRINUSE) {
			return true
		}
	}
	return false
}

func (c *CallbackPreflight) owner(ctx context.Context) (PortOwner, error) {
	if c.Owner != nil {
		return c.Owner(ctx, c.Port)
	}
	return lsofOwner(ctx, c.Port)
}

func lsofOwner(ctx context.Context, port int) (PortOwner, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return PortOwner{}, errors.ErrUnsupported
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "lsof", "-nP", fmt.Sprintf("-iTCP:%d", port), "-sTCP:LISTEN", "-Fpc").Output()
	if owner, ok := ParseLsofOwner(string(out)); ok {
		return owner, nil
	}
	if err == nil {
		err = errors.New("no listener reported")
	}
	return PortOwner{}, err
}

// ParseLsofOwner reads the first process of `lsof -Fpc` output.
func ParseLsofOwner(out string) (PortOwner, bool) {
	var owner PortOwner
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			if owner.PID != 0 {
				return owner, owner.Command != ""
			}
			owner.PID, _ = strconv.Atoi(line[1:])
		case 'c':
			if owner.Command == "" {
				owner.Command = line[1:]
			}
		}
	}
	return owner, owner.PID != 0 && owner.Command != ""
}
