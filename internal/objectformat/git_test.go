package objectformat

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNullOID(t *testing.T) {
	nullRE := regexp.MustCompile(`\A0+\z`)

	sha1 := ObjectFormat("sha1")
	require.Equal(t, len(sha1.NullOID()), 40)
	require.Regexp(t, nullRE, sha1.NullOID())

	sha256 := ObjectFormat("sha256")
	require.Equal(t, len(sha256.NullOID()), 64)
	require.Regexp(t, nullRE, sha256.NullOID())
}

func TestGetObjectFormat(t *testing.T) {
	of, err := GetObjectFormat("../spokes/testdata/lots-of-refs.git")
	require.NoError(t, err)
	require.Equal(t, of, ObjectFormat("sha1"))
}
