//go:build integration
// +build integration

package integration

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"testing"
)

func TestMain(m *testing.M) {
	if err := os.Chdir(".."); err != nil {
		log.Fatal("unable to chdir to project base directory", err)
	}

	// Setup a folder that will be used as Git repo
	if err := exec.Command("mkdir", "/tmp/local").Run(); err != nil {
		log.Fatal(err)
	}

	if err := os.Chdir("/tmp/local"); err != nil {
		log.Fatal("unable to chdir new local Git Repo", err)
	}

	if err := exec.Command("git", "init").Run(); err != nil {
		log.Fatal(err)
	}

	// add some content to our repo and commit it
	err := os.WriteFile("README.md", []byte("A simple README.md file"), 0644)
	if err != nil {
		log.Fatal("unable to create a README.md file in the Git repo", err)
	}
	if err := exec.Command("git", "add", ".").Run(); err != nil {
		log.Fatal(err)
	}
	if err := exec.Command("git", "commit", "--message", "First commit").Run(); err != nil {
		log.Fatal(err)
	}

	// setup a remote (assuming it's going to be localted in /tmp/remote
	if err := exec.Command("git", "remote", "add", "r", "/tmp/remote").Run(); err != nil {
		log.Fatal(err)
	}

	// Setup a folder that will be used as Git remote of our /tmp/local repo
	if err := exec.Command("mkdir", "/tmp/remote").Run(); err != nil {
		log.Fatal(err)
	}

	if err := os.Chdir("/tmp/remote"); err != nil {
		log.Fatal("unable to chdir to project base directory", err)
	}

	if err := exec.Command("git", "init", "--quiet", "--template=.", "--bare").Run(); err != nil {
		log.Fatal(err)
	}

	r := m.Run()

	// Clean the environment before exiting
	os.RemoveAll("/tmp/remote")
	os.RemoveAll("/tmp/local")
	os.Exit(r)
}

func TestSuccessfulPush(t *testing.T) {
	if err := os.Chdir("/tmp/local"); err != nil {
		t.Fatal("unable to chdir to our local Git repo", err)
	}

	cmd := exec.Command("git", "push", "--receive-pack=spokes-receive-pack", "r", "master")

	output, err := cmd.CombinedOutput()
	fmt.Println("Results:" + string(output))
	if err != nil {
		t.Fatal(string(output))
	}
}
