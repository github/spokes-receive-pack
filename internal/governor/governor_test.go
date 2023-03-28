package governor

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUpdate(t *testing.T) {
	var buf bytes.Buffer

	err := update(&buf, updateData{Program: "test-prog"})

	assert.NoError(t, err)
	assert.Equal(t, `{"command":"update","data":{"program":"test-prog"}}`, buf.String())
}

func TestSchedule(t *testing.T) {
	examples := []struct {
		response      string
		expectedError error
	}{
		{
			response: "continue\n",
		},
		{
			response:      "wait 100\n",
			expectedError: WaitError{Duration: 100 * time.Second},
		},
		{
			response:      "fail Too Busy\n",
			expectedError: FailError{Reason: "Too Busy"},
		},
		{
			response:      "",
			expectedError: io.EOF,
		},
		{
			response:      "\n",
			expectedError: errors.New(`unexpected response "" from governor`),
		},
	}

	for _, ex := range examples {
		t.Run(strings.TrimSpace(ex.response), func(t *testing.T) {
			var toGov bytes.Buffer
			fromGov := bufio.NewReader(strings.NewReader(ex.response))

			err := schedule(fromGov, &toGov)

			assert.Equal(t, ex.expectedError, err)
			assert.Equal(t, `{"command":"schedule"}`, toGov.String())
		})
	}
}
