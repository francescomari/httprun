package httprun

import (
	"context"
	"net"
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
func (s Server) ListenAndServe(ctx context.Context) error {
	return s.run(ctx, func() error {
		return s.HTTPServer.ListenAndServe()
	})
}

// ListenAndServeTLS has the same semantics of the ListenAndServeTLS method of
// http.Server. In addition, ListenAndServeTLS will terminate after a graceful
// shutdown when the given context is cancelled.
func (s Server) ListenAndServeTLS(ctx context.Context, certFile, keyFile string) error {
	return s.run(ctx, func() error {
		return s.HTTPServer.ListenAndServeTLS(certFile, keyFile)
	})
}

// Serve has the same semantics of the Serve method of http.Server. In addition,
// Serve will terminate after a graceful shutdown when the given context is
// cancelled.
func (s Server) Serve(ctx context.Context, l net.Listener) error {
	return s.run(ctx, func() error {
		return s.HTTPServer.Serve(l)
	})
}

// ServeTLS has the same semantics of the ServeTLS method of http.Server. In
// addition, ServeTLS will terminate after a graceful shutdown when the given
// context is cancelled.
func (s Server) ServeTLS(ctx context.Context, l net.Listener, certFile, keyFile string) error {
	return s.run(ctx, func() error {
		return s.HTTPServer.ServeTLS(l, certFile, keyFile)
	})
}

func (s Server) run(ctx context.Context, serve func() error) error {
	var (
		serveDone  = make(chan struct{})
		serveError = make(chan error, 1)
	)

	defer func() {
		<-serveDone
	}()

	go func() {
		defer close(serveDone)
		serveError <- serve()
	}()

	select {
	case err := <-serveError:
		return err
	case <-ctx.Done():
		return s.shutdownGracefully()
	}
}

func (s Server) shutdownGracefully() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.ShutdownTimeout)
	defer cancel()

	return s.HTTPServer.Shutdown(ctx)
}
