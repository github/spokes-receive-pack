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
