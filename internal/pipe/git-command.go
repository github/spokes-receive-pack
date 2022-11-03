package pipe

import (
	"os/exec"
)

// gitCommand is the thing we run for GitCommands. Calling CacheGitCommandPath
// will overwrite it with the full path to git, if possible.
var gitCommand = "git"

// CacheGitCommandPath is called during app startup to get the full path to
// "git" so that we don't need to do it every time a pipeline runs.
func CacheGitCommandPath() error {
	path, err := exec.LookPath("git")
	if err != nil {
		return err
	}
	gitCommand = path
	return nil
}

// GitCommand returns a `Stage` representing a `git` subcommand. It assumes
// that the pipeline's working directory is a GIT_DIR.
func GitCommand(subcommand string, args ...string) Stage {
	stage, _ := GitCommand2(subcommand, args...)
	return stage
}

// GitCommand2 returns a `Stage` representing a `git` subcommand. It assumes
// that the pipeline's working directory is a GIT_DIR. It also returns the
// *exec.Cmd for further customization, though this should be done with care!
func GitCommand2(subcommand string, args ...string) (Stage, *exec.Cmd) {
	if len(subcommand) == 0 {
		panic("attempt to add empty git command to pipeline")
	}

	cmdArgs := make([]string, 0, len(args)+3)
	cmdArgs = append(cmdArgs, "--git-dir", ".")
	cmdArgs = append(cmdArgs, subcommand)
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.Command(gitCommand, cmdArgs...)
	return CommandStage("git-"+subcommand, cmd), cmd
}

// PorcelainGitCommand returns a `Stage` representing a `git` subcommand that
// doesn't assume it's running in the repository's GIT_DIR.
func PorcelainGitCommand(subcommand string, args ...string) Stage {
	if len(subcommand) == 0 {
		panic("attempt to add empty git command to pipeline")
	}

	cmdArgs := append([]string{subcommand}, args...)
	cmd := exec.Command(gitCommand, cmdArgs...)
	return CommandStage("git-"+subcommand, cmd)
}
