package httprun_test

import (
	"context"
	"errors"
	"net"
	"runtime"
	"testing"

	"github.com/francescomari/httprun"
)

var (
	errServe    = errors.New("listen and serve")
	errShutdown = errors.New("shutdown")
)

type serverRunner func(context.Context, httprun.Server) []error

type testRunner func(*testing.T, serverRunner)

func TestServer(t *testing.T) {
	tests := []struct {
		name      string
		runServer serverRunner
	}{
		{
			name: "listen-and-serve",
			runServer: func(ctx context.Context, s httprun.Server) []error {
				return s.ListenAndServe(ctx)
			},
		},
		{
			name: "listen-and-serve-tls",
			runServer: func(ctx context.Context, s httprun.Server) []error {
				return s.ListenAndServeTLS(ctx, "cert", "key")
			},
		},
		{
			name: "serve",
			runServer: func(ctx context.Context, s httprun.Server) []error {
				return s.Serve(ctx, nil)
			},
		},
		{
			name: "serve-tls",
			runServer: func(ctx context.Context, s httprun.Server) []error {
				return s.ServeTLS(ctx, nil, "cert", "key")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testServerWithRunner(t, test.runServer)
		})
	}
}

func testServerWithRunner(t *testing.T, runServer serverRunner) {
	tests := []struct {
		name    string
		runTest testRunner
	}{
		{
			name:    "serve",
			runTest: testServe,
		},
		{
			name:    "errors",
			runTest: testErrors,
		},
		{
			name:    "setup-error",
			runTest: testSetupError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checkNoGoroutineLeaks(t)
			test.runTest(t, runServer)
		})
	}
}

func testServe(t *testing.T, runServer serverRunner) {
	ctx, server := newMockServerStartingAndStopping(t)

	s := httprun.Server{
		HTTPServer: server,
	}

	errs := runServer(ctx, s)

	checkErrorsLength(t, errs, 0)
}

func testErrors(t *testing.T, runServer serverRunner) {
	ctx, server := newMockServerStartingAndStoppingWithErrors(t)

	s := httprun.Server{
		HTTPServer: server,
	}

	errs := runServer(ctx, s)

	checkErrorsLength(t, errs, 2)
	checkErrorsContain(t, errs, errServe)
	checkErrorsContain(t, errs, errShutdown)
}

func testSetupError(t *testing.T, runServer serverRunner) {
	server := newMockServerNotStarting(t)

	s := httprun.Server{
		HTTPServer: server,
	}

	errs := runServer(context.Background(), s)

	checkErrorsLength(t, errs, 1)
	checkErrorsContain(t, errs, errServe)
}

type mockServer struct {
	doListenAndServe    func() error
	doListenAndServeTLS func(string, string) error
	doServe             func(net.Listener) error
	doServeTLS          func(net.Listener, string, string) error
	doShutdown          func(ctx context.Context) error
}

func (s mockServer) ListenAndServe() error {
	return s.doListenAndServe()
}

func (s mockServer) ListenAndServeTLS(certFile, keyFile string) error {
	return s.doListenAndServeTLS(certFile, keyFile)
}

func (s mockServer) Serve(l net.Listener) error {
	return s.doServe(l)
}

func (s mockServer) ServeTLS(l net.Listener, certFile, keyFile string) error {
	return s.doServeTLS(l, certFile, keyFile)
}

func (s mockServer) Shutdown(ctx context.Context) error {
	return s.doShutdown(ctx)
}

func newMockServerStartingAndStopping(t *testing.T) (context.Context, httprun.HTTPServer) {
	t.Helper()

	var (
		serveCalled    = make(chan struct{})
		shutdownCalled = make(chan struct{})
	)

	serve := func() error {
		close(serveCalled)
		<-shutdownCalled
		return nil
	}

	server := mockServer{
		doListenAndServe: func() error {
			return serve()
		},
		doListenAndServeTLS: func(string, string) error {
			return serve()
		},
		doServe: func(net.Listener) error {
			return serve()
		},
		doServeTLS: func(net.Listener, string, string) error {
			return serve()
		},
		doShutdown: func(context.Context) error {
			close(shutdownCalled)
			return nil
		},
	}

	return newContextForChannel(t, serveCalled), server
}

func newMockServerStartingAndStoppingWithErrors(t *testing.T) (context.Context, httprun.HTTPServer) {
	t.Helper()

	var (
		serveCalled    = make(chan struct{})
		shutdownCalled = make(chan struct{})
	)

	serve := func() error {
		close(serveCalled)
		<-shutdownCalled
		return errServe
	}

	server := mockServer{
		doListenAndServe: func() error {
			return serve()
		},
		doListenAndServeTLS: func(string, string) error {
			return serve()
		},
		doServe: func(net.Listener) error {
			return serve()
		},
		doServeTLS: func(net.Listener, string, string) error {
			return serve()
		},
		doShutdown: func(context.Context) error {
			close(shutdownCalled)
			return errShutdown
		},
	}

	return newContextForChannel(t, serveCalled), server
}

func newMockServerNotStarting(t *testing.T) httprun.HTTPServer {
	t.Helper()

	return mockServer{
		doListenAndServe: func() error {
			return errServe
		},
		doListenAndServeTLS: func(string, string) error {
			return errServe
		},
		doServe: func(net.Listener) error {
			return errServe
		},
		doServeTLS: func(net.Listener, string, string) error {
			return errServe
		},
		doShutdown: func(ctx context.Context) error {
			t.Fatal("Shutdown should not be called")
			return nil
		},
	}
}

func newContextForChannel(t *testing.T, done <-chan struct{}) context.Context {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	t.Cleanup(cancel)

	go func() {
		<-done
		cancel()
	}()

	return ctx
}

func checkErrorsLength(t *testing.T, errors []error, length int) {
	t.Helper()

	if len(errors) != length {
		t.Fatalf("expected %d errors, got %d", len(errors), length)
	}
}

func checkErrorsContain(t *testing.T, errors []error, target error) {
	t.Helper()

	for _, err := range errors {
		if err == target {
			return
		}
	}

	t.Fatalf("error '%v' not found", target)
}

func checkNoGoroutineLeaks(t *testing.T) {
	t.Helper()

	numGoroutinesBefore := runtime.NumGoroutine()

	t.Cleanup(func() {
		leaked := runtime.NumGoroutine() - numGoroutinesBefore

		switch {
		case leaked == 1:
			t.Fatalf("one goroutine leaked")
		case leaked > 1:
			t.Fatalf("%d goroutines leaked", leaked)
		}
	})
}
