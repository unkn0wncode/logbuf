package logbuf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewValidation(t *testing.T) {
	_, err := NewSQliteBuffer(0, 0, ":memory:")
	require.Error(t, err, "expected error when both constraints zero")
}

func TestWriteAndDump(t *testing.T) {
	fp := filepath.Join(t.TempDir(), "lb.db")
	lb, err := NewSQliteBuffer(10, 0, fp)
	require.NoError(t, err)
	defer lb.Clear()

	want := []string{"first entry", "second entry"}
	for _, e := range want {
		require.NoError(t, lb.WriteString(e))
	}

	got, err := lb.Dump()
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestMaxLinesRetention(t *testing.T) {
	fp := filepath.Join(t.TempDir(), "lb.db")
	lb, err := NewSQliteBuffer(2, 0, fp)
	require.NoError(t, err)
	defer lb.Clear()

	for _, e := range []string{"a", "b", "c"} {
		require.NoError(t, lb.WriteString(e))
	}

	got, err := lb.Dump()
	require.NoError(t, err)
	require.Equal(t, []string{"b", "c"}, got)
}

func TestClear(t *testing.T) {
	fp := filepath.Join(t.TempDir(), "lb.db")
	lb, err := NewSQliteBuffer(10, 0, fp)
	require.NoError(t, err)
	defer lb.Clear()

	require.NoError(t, lb.WriteString("to be removed"))
	require.NoError(t, lb.Clear())

	_, statErr := os.Stat(fp)
	require.True(t, os.IsNotExist(statErr), "database file should be removed after Clear")

	entries, err := lb.Dump()
	require.NoError(t, err)
	require.Len(t, entries, 0)

	_, err = os.Stat(fp)
	require.NoError(t, err, "database file should be recreated after Dump")
}
