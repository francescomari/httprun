package httprun

import (
	"context"
	"net"
	"net/http"
	"time"
)

// HTTPServer is an HTTP server that can be started and shut down. HTTPServer
// mimics the interface of http.Server. Every method in this interface has the
// same semantics as the corresponding methods in http.Server.
type HTTPServer interface {
	ListenAndServe() error
	ListenAndServeTLS(certFile, keyFile string) error
	Serve(l net.Listener) error
	ServeTLS(l net.Listener, certFile, keyFile string) error
	Shutdown(ctx context.Context) error
}

// Server is a server that can be shut down gracefully. Every method defined on
// Server has the same semantics as the method with the same name in
// http.Server, with additional behaviour to guarantee a graceful shutdown of
// the server.
type Server struct {
	HTTPServer      HTTPServer
	ShutdownTimeout time.Duration
}

// ListenAndServe has the same semantics of the ListenAndServe method of
// http.Server. In addition, ListenAndServe will terminate after a graceful
// shutdown when the given context is cancelled.
func (s Server) ListenAndServe(ctx context.Context) []error {
	return s.run(ctx, func() error {
		return s.HTTPServer.ListenAndServe()
	})
}

// ListenAndServeTLS has the same semantics of the ListenAndServeTLS method of
// http.Server. In addition, ListenAndServeTLS will terminate after a graceful
// shutdown when the given context is cancelled.
func (s Server) ListenAndServeTLS(ctx context.Context, certFile, keyFile string) []error {
	return s.run(ctx, func() error {
		return s.HTTPServer.ListenAndServeTLS(certFile, keyFile)
	})
}

// Serve has the same semantics of the Serve method of http.Server. In addition,
// Serve will terminate after a graceful shutdown when the given context is
// cancelled.
func (s Server) Serve(ctx context.Context, l net.Listener) []error {
	return s.run(ctx, func() error {
		return s.HTTPServer.Serve(l)
	})
}

// ServeTLS has the same semantics of the ServeTLS method of http.Server. In
// addition, ServeTLS will terminate after a graceful shutdown when the given
// context is cancelled.
func (s Server) ServeTLS(ctx context.Context, l net.Listener, certFile, keyFile string) []error {
	return s.run(ctx, func() error {
		return s.HTTPServer.ServeTLS(l, certFile, keyFile)
	})
}

func (s Server) run(ctx context.Context, serve func() error) []error {
	var (
		errors       = make(chan error, 2)
		shutdownDone = make(chan struct{})
		serveDone    = make(chan struct{})
	)

	go func() {
		defer close(serveDone)

		errors <- serve()
	}()

	go func() {
		defer close(shutdownDone)

		select {
		case <-serveDone:
			return
		case <-ctx.Done():
			errors <- s.shutdownGracefully()
		}
	}()

	go func() {
		defer close(errors)

		<-shutdownDone
		<-serveDone
	}()

	var result []error

	for err := range errors {
		if err != nil {
			result = append(result, err)
		}
	}

	return result
}

func (s Server) shutdownGracefully() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.ShutdownTimeout)
	defer cancel()

	return s.HTTPServer.Shutdown(ctx)
}

// HandleErrors simplifies the handling of errors returned by the Server.
// HandleErrors masks out cancellation errors from the context package and
// http.ErrServerClosed, which is always returned by  most of the functions in
// http.Server. HandleErrors returns the first non-nil error after masking out
// the unwanted ones. If all the errors were masked out, or if no errors were
// passed as input, HandleErrors returns nil.
func HandleErrors(errors []error) error {
	for _, err := range errors {
		if err := handleError(err); err != nil {
			return err
		}
	}
	return nil
}

func handleError(err error) error {
	switch err {
	case context.DeadlineExceeded:
		return nil
	case context.Canceled:
		return nil
	case http.ErrServerClosed:
		return nil
	default:
		return err
	}
}
