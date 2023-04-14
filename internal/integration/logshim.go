//go:build integration

package integration

import "testing"

type testLogWriter struct {
	t *testing.T
}

func (w *testLogWriter) Write(data []byte) (int, error) {
	w.t.Logf("%s", data)
	return len(data), nil
}
