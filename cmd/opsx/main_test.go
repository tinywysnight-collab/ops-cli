package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSignalContextCancelFuncCancelsRootContext(t *testing.T) {
	ctx, stop := signalContext(context.Background())
	stop()
	require.ErrorIs(t, ctx.Err(), context.Canceled)
}
