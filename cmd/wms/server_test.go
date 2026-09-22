package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServeStopsAllServersOnListenerFailure(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	servers := []*http.Server{
		{Addr: "127.0.0.1:0", ReadHeaderTimeout: time.Second},
		{Addr: occupied.Addr().String(), ReadHeaderTimeout: time.Second},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := serve(ctx, time.Second, servers...); err == nil {
		t.Fatal("listener failure was hidden")
	}
	for _, srv := range servers {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("server was not stopped: %v", err)
		}
	}
}

func TestServeStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := serve(ctx, time.Second, &http.Server{Addr: "127.0.0.1:0", ReadHeaderTimeout: time.Second}); err != nil {
		t.Fatal(err)
	}
}

func TestServeClosesActiveConnectionsOnShutdownTimeout(t *testing.T) {
	address := make(chan string, 1)
	started := make(chan struct{})
	handlerDone := make(chan struct{})
	srv := &http.Server{
		Addr:              "127.0.0.1:0",
		ReadHeaderTimeout: time.Second,
		BaseContext:       func(l net.Listener) context.Context { address <- l.Addr().String(); return context.Background() },
		Handler: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			close(started)
			<-r.Context().Done()
			close(handlerDone)
		}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.Cleanup(func() { _ = srv.Close() })
	result := make(chan error, 1)
	go func() { result <- serve(ctx, 10*time.Millisecond, srv) }()
	clientDone := make(chan struct{})
	client := &http.Client{Timeout: 3 * time.Second}
	go func() {
		defer close(clientDone)
		resp, err := client.Get("http://" + <-address)
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("got %v, want shutdown timeout", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop")
	}
	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("active connection was not closed")
	}
	<-clientDone
}
