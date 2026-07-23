package zitadel

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestTokenHTTPClientBounded(t *testing.T) {
	c, err := tokenHTTPClient(nil)
	require.NoError(t, err)
	assert.Equal(t, tokenHTTPTimeout, c.Timeout,
		"token requests must carry a hard timeout: an unbounded token POST on a dead connection wedges every RPC")
	assert.NotSame(t, http.DefaultTransport, c.Transport,
		"must not mutate or share the process-global default transport")
}

func TestTokenHTTPClientWrap(t *testing.T) {
	wrapped := false
	c, err := tokenHTTPClient(func(inner http.RoundTripper) http.RoundTripper {
		wrapped = true
		return &instanceHeaderTransport{instanceHost: "example.test", inner: inner}
	})
	require.NoError(t, err)
	assert.True(t, wrapped)
	_, ok := c.Transport.(*instanceHeaderTransport)
	assert.True(t, ok, "wrap decorator must become the client transport")
}

func TestDefaultTimeoutUnaryInterceptorAddsDeadline(t *testing.T) {
	interceptor := defaultTimeoutUnaryInterceptor(time.Minute)

	var got time.Time
	var ok bool
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		got, ok = ctx.Deadline()
		return nil
	}

	err := interceptor(context.Background(), "/svc/Method", nil, nil, nil, invoker)
	require.NoError(t, err)
	require.True(t, ok, "deadline-less calls must get the default deadline")
	assert.InDelta(t, time.Minute.Seconds(), time.Until(got).Seconds(), 5)
}

func TestDefaultTimeoutUnaryInterceptorKeepsCallerDeadline(t *testing.T) {
	interceptor := defaultTimeoutUnaryInterceptor(time.Minute)

	callerCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	callerDeadline, _ := callerCtx.Deadline()

	var got time.Time
	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		got, _ = ctx.Deadline()
		return nil
	}

	err := interceptor(callerCtx, "/svc/Method", nil, nil, nil, invoker)
	require.NoError(t, err)
	assert.Equal(t, callerDeadline, got, "existing deadlines must pass through untouched")
}
