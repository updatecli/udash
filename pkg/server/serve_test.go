package server

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeFinishesInFlightRequestsOnShutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	started := make(chan struct{})
	srv := &http.Server{Addr: addr, ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	})}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, srv, 5*time.Second) }()

	status := make(chan int, 1)
	go func() {
		for {
			res, err := http.Get("http://" + addr)
			if err == nil {
				res.Body.Close()
				status <- res.StatusCode
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	select {
	case <-started:
	case err := <-done:
		t.Fatalf("serve returned before the handler started: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("the handler did not start")
	}
	cancel()

	assert.Equal(t, http.StatusNoContent, <-status, "the in-flight request must complete")
	assert.NoError(t, <-done)

	_, err = http.Get("http://" + addr)
	assert.Error(t, err, "no new connection may be accepted after shutdown")
}

func TestServeReturnsListenErrors(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	err = serve(context.Background(), &http.Server{Addr: ln.Addr().String(), ReadHeaderTimeout: time.Second}, time.Second)
	assert.ErrorContains(t, err, "address already in use")
}

func TestServeCancelsRequestsPastTheShutdownTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	started := make(chan struct{})
	canceled := make(chan struct{})
	srv := &http.Server{Addr: addr, ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(canceled)
	})}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, srv, 200*time.Millisecond) }()

	go func() {
		for {
			res, err := http.Get("http://" + addr)
			if err == nil {
				res.Body.Close()
			}
			select {
			case <-started:
				return
			default:
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()

	select {
	case <-started:
	case err := <-done:
		t.Fatalf("serve returned before the handler started: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("the handler did not start")
	}
	cancel()

	assert.ErrorIs(t, <-done, context.DeadlineExceeded)

	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("the request still running past the shutdown timeout was not canceled")
	}
}
