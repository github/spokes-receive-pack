package governor

import "testing"

func TestReadSockstat(t *testing.T) {
	examples := []struct {
		label    string
		environ  []string
		expected updateData
	}{
		{
			label: "ignored environment",
			environ: []string{
				"HTTP_X_SOCKSTAT_repo_name=ignored",
				"REMOTE_ADDR=ignored",
				"GIT_SOCKSTAT_VAR_ignored=ignored",
				"GIT_SOCKSTAT_VAR_user_id=ignored",
				"GIT_SOCKSTAT_VAR_network_id=bool:false",
			},
		},
		{
			label: "all the fields",
			environ: []string{
				"GIT_SOCKSTAT_VAR_repo_name=a/b",
				"GIT_SOCKSTAT_VAR_repo_id=uint:1",
				"GIT_SOCKSTAT_VAR_network_id=uint:2",
				"GIT_SOCKSTAT_VAR_user_id=uint:3",
				"GIT_SOCKSTAT_VAR_real_ip=1.2.3.4",
				"GIT_SOCKSTAT_VAR_request_id=AAAA:BBBB:CCCC-DDDD",
				"GIT_SOCKSTAT_VAR_user_agent=Testing/1.2.3 xyz=blah",
				"GIT_SOCKSTAT_VAR_features=random",
				"GIT_SOCKSTAT_VAR_via=git",
				"GIT_SOCKSTAT_VAR_ssh_connection=ssh-anything",
				"GIT_SOCKSTAT_VAR_babeld=babeld-anything",
				"GIT_SOCKSTAT_VAR_git_protocol=http",
				"GIT_SOCKSTAT_VAR_pubkey_verifier_id=uint:10",
				"GIT_SOCKSTAT_VAR_pubkey_creator_id=uint:11",
			},
			expected: updateData{
				RepoName:         "a/b",
				RepoID:           1,
				NetworkID:        2,
				UserID:           3,
				RealIP:           "1.2.3.4",
				RequestID:        "AAAA:BBBB:CCCC-DDDD",
				UserAgent:        "Testing/1.2.3 xyz=blah",
				Features:         "random",
				Via:              "git",
				SSHConnection:    "ssh-anything",
				Babeld:           "babeld-anything",
				GitProtocol:      "http",
				PubkeyVerifierID: 10,
				PubkeyCreatorID:  11,
			},
		},
	}

	for _, ex := range examples {
		actual := readSockstat(ex.environ)
		if actual != ex.expected {
			t.Errorf("%s: incorrect output\nexpected: %+v\nactual:   %+v\n", ex.label, ex.expected, actual)
		}
	}
}

func TestUint32(t *testing.T) {
	examples := []struct {
		input  string
		output uint32
	}{
		{"", 0},
		{"123", 0},
		{"abc", 0},
		{"bool:true", 0},
		{"bool:false", 0},
		{"uint:-1", 0},
		{"uint:1", 1},
		{"uint:4294967295", 4294967295},
		{"uint:4294967296", 0},
		{"uint:4294967297", 0},
		{"uint:abc", 0},
		{"uint: 1", 0},
		{"uint:1 ", 0},
	}

	for _, ex := range examples {
		actual := sockstatUint32(ex.input)
		if actual != ex.output {
			t.Errorf("sockstatUint32(%q): expected %d, but was %d", ex.input, ex.output, actual)
		}
	}
}

func TestString(t *testing.T) {
	examples := []struct {
		input  string
		output string
	}{
		{"", ""},
		{"123", "123"},
		{"abc", "abc"},
		{"bool:true", "true"},
		{"bool:false", "false"},
		{"uint:-1", "-1"},
		{"uint:1", "1"},
		{"uint:4294967295", "4294967295"},
		{"uint:4294967296", "4294967296"},
		{"bool:uint:anything", "uint:anything"},
		{"uint:bool:anything", "bool:anything"},
		{"anything:uint:bool", "anything:uint:bool"},
	}

	for _, ex := range examples {
		actual := sockstatString(ex.input)
		if actual != ex.output {
			t.Errorf("sockstatUint32(%q): expected %q, but was %q", ex.input, ex.output, actual)
		}
	}
}
