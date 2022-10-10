//go:build integration

package integration

import (
	"log"
	"os"
	"os/exec"
	"testing"
)

var localRepo, remoteRepo string

func TestMain(m *testing.M) {

	var err error

	// set up a folder that will be used as a "local" Git repo
	localRepo, err = os.MkdirTemp("", "local")
	if err != nil {
		log.Fatal("Unable to create the local repository directory", err)
	}

	// set up a folder that will be used as a "remote" Git repo
	remoteRepo, err = os.MkdirTemp("", "remote")
	if err != nil {
		log.Fatal("Unable to create the remote repository directory", err)
	}

	if err := os.Chdir(localRepo); err != nil {
		log.Fatal("unable to chdir new local Git Repo", err)
	}

	if err := exec.Command("git", "init").Run(); err != nil {
		log.Fatal(err)
	}

	// Configure our local repo
	if err := exec.Command("git", "config", "user.email", "spokes-receive-pack@github.com").Run(); err != nil {
		log.Fatal(err)
	}
	if err := exec.Command("git", "config", "user.name", "spokes-receive-pack").Run(); err != nil {
		log.Fatal(err)
	}

	// add some content to our repo and commit it
	err = os.WriteFile("README.md", []byte("A simple README.md file"), 0644)
	if err != nil {
		log.Fatal("unable to create a README.md file in the Git repo", err)
	}
	if err := exec.Command("git", "add", ".").Run(); err != nil {
		log.Fatal(err)
	}
	if err := exec.Command("git", "commit", "--message", "First commit").Run(); err != nil {
		log.Fatal(err)
	}

	// configure the remote
	if err := exec.Command("git", "remote", "add", "r", remoteRepo).Run(); err != nil {
		log.Fatal(err)
	}

	if err := os.Chdir(remoteRepo); err != nil {
		log.Fatal("unable to chdir to project base directory", err)
	}

	if err := exec.Command("git", "init", "--quiet", "--template=.", "--bare").Run(); err != nil {
		log.Fatal(err)
	}

	r := m.Run()

	// Clean the environment before exiting
	if err := os.RemoveAll(remoteRepo); err != nil {
		log.Fatal(err)
	}

	if err := os.RemoveAll(localRepo); err != nil {
		log.Fatal(err)
	}

	os.Exit(r)
}

func TestSuccessfulPush(t *testing.T) {
	if err := os.Chdir(localRepo); err != nil {
		t.Fatal("unable to chdir to our local Git repo", err)
	}

	if err := exec.Command("git", "push", "--receive-pack=spokes-receive-pack", "r", "master").Run(); err != nil {
		t.Fatal(err)
	}
}
