package gcp_test

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/k0wl0n/gctx/pkg/provider/gcp"
)

func holdPort(t *testing.T) (net.Listener, int) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return l, l.Addr().(*net.TCPAddr).Port
}

func TestPreflightFreePortReturnsAtOnce(t *testing.T) {
	check := freePortPreflight(t)
	var out strings.Builder
	report := check.Check(context.Background(), &out)
	if report.Busy || out.Len() != 0 {
		t.Errorf("report %+v out %q", report, out.String())
	}
}

func TestPreflightWaitsForRelease(t *testing.T) {
	l, port := holdPort(t)
	go func() { time.Sleep(60 * time.Millisecond); l.Close() }()
	check := &gcp.CallbackPreflight{Port: port, Wait: 2 * time.Second, Poll: 10 * time.Millisecond}
	var out strings.Builder
	start := time.Now()
	report := check.Check(context.Background(), &out)
	if !report.Busy || report.StillBusy || time.Since(start) > time.Second {
		t.Errorf("report %+v after %v", report, time.Since(start))
	}
	if out.Len() != 0 {
		t.Errorf("warned although released: %q", out.String())
	}
}

func TestPreflightStillBusyWarnsWithOwner(t *testing.T) {
	l, port := holdPort(t)
	defer l.Close()
	check := &gcp.CallbackPreflight{
		Port: port, Wait: 30 * time.Millisecond, Poll: 10 * time.Millisecond,
		Owner: func(context.Context, int) (gcp.PortOwner, error) {
			return gcp.PortOwner{Command: "python3", PID: 4242}, nil
		},
	}
	var out strings.Builder
	report := check.Check(context.Background(), &out)
	if !report.StillBusy || report.Owner.PID != 4242 {
		t.Errorf("report %+v", report)
	}
	want := "is in use by python3 (pid 4242); gcloud's login callback needs it — close it or retry"
	if !strings.Contains(out.String(), want) || strings.Count(out.String(), "\n") != 1 {
		t.Errorf("warning %q", out.String())
	}
}

func TestPreflightWithoutOwnerLookupStillWarns(t *testing.T) {
	l, port := holdPort(t)
	defer l.Close()
	check := &gcp.CallbackPreflight{
		Port: port, Wait: 20 * time.Millisecond, Poll: 10 * time.Millisecond,
		Owner: func(context.Context, int) (gcp.PortOwner, error) {
			return gcp.PortOwner{}, errors.New("lsof: not found")
		},
	}
	var out strings.Builder
	check.Check(context.Background(), &out)
	if !strings.Contains(out.String(), "is in use; gcloud's login callback needs it") {
		t.Errorf("warning %q", out.String())
	}
}

func TestPreflightStopsOnCancel(t *testing.T) {
	l, port := holdPort(t)
	defer l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(30 * time.Millisecond); cancel() }()
	check := &gcp.CallbackPreflight{Port: port, Wait: 5 * time.Second, Poll: 10 * time.Millisecond}
	start := time.Now()
	check.Check(ctx, &strings.Builder{})
	if time.Since(start) > time.Second {
		t.Errorf("cancel took %v", time.Since(start))
	}
}

func TestParseLsofOwner(t *testing.T) {
	owner, ok := gcp.ParseLsofOwner("p812\ncgcloud\nf5\n")
	if !ok || owner.Command != "gcloud" || owner.PID != 812 {
		t.Errorf("owner %+v ok %v", owner, ok)
	}
	if _, ok := gcp.ParseLsofOwner(""); ok {
		t.Error("empty output parsed")
	}
}

func freePortPreflight(t *testing.T) *gcp.CallbackPreflight {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return &gcp.CallbackPreflight{Port: port, Wait: 50 * time.Millisecond, Poll: 10 * time.Millisecond}
}
