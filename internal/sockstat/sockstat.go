package sockstat

import (
	"os"
	"strconv"
	"strings"
)

// Prefix is the prefix that all sockstat environment variable names must have.
const Prefix = "GIT_SOCKSTAT_VAR_"

// GetString looks up the given sockstat var name in the environment and
// interprets it as a string. If the var isn't present, the empty string is
// returned.
func GetString(name string) string {
	return StringValue(os.Getenv(Prefix + name))
}

// GetUint32 looks up the given sockstat var name in the environment and
// interprets it as a uint32. If the var isn't present or isn't a uint32, 0 is
// returned.
func GetUint32(name string) uint32 {
	return Uint32Value(os.Getenv(Prefix + name))
}

// StringValue returns the string version of the given sockstat var. For the
// most part, this means just returning the given string. However, if the input
// has a uint or bool prefix, strip that off so that it looks like we parsed
// the value and then stringified it.
func StringValue(s string) string {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) == 2 && (parts[0] == "uint" || parts[0] == "bool") {
		return parts[1]
	}
	return s
}

// Uint32Value parses a string like "uint32:123" and returns the parsed uint32
// like 123. If the prefix is missing or the value isn't a uint32, return 0.
func Uint32Value(s string) uint32 {
	s, ok := strings.CutPrefix(s, "uint:")
	if !ok {
		return 0
	}
	val, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0
	}
	return uint32(val)
}
