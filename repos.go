package main

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"os"
	"strings"
)

func cloneRepo(ctx *context.Context, cli *client.Client, repo Repository, WorkingDirectory string, VolumeName string) (err error) {
	var ref string
	if len(repo.Branch) > 0 {
		ref = repo.Branch
	}
	if len(ref) == 0 && len(repo.Tag) > 0 {
		ref = repo.Tag
	}
	if len(ref) == 0 || len(repo.Commit) > 0 {
		ref = repo.Commit
	}
	if len(ref) > 0 {
		fmt.Printf("Ignoring shallow because branch was specified.\n")
		repo.Shallow = false
	}

	commands := []string{"clone"}
	if repo.Shallow {
		commands = append(commands, "--depth", "1")
	}
	commands = append(commands, repo.Location)
	if len(repo.Directory) > 0 {
		commands = append(commands, repo.Directory)
	}

	environment := []string{
		"GIT_SSH_COMMAND=ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no",
	}
	bindMounts := []mount.Mount{}
	for _, envVar := range os.Environ() {
		pair := strings.Split(envVar, "=")
		if pair[0] == "SSH_AUTH_SOCK" {
			environment = append(
				environment,
				envVar,
			)
			bindMounts = append(
				bindMounts,
				mount.Mount{
					Type:   mount.TypeBind,
					Source: pair[1],
					Target: pair[1],
				},
			)
		}
	}
	err = runForegroundContainer(
		ctx,
		cli,
		"alpine/git",
		commands,
		[]string{},
		"",
		environment,
		WorkingDirectory,
		"",
		VolumeName,
		bindMounts,
		false,
		os.Stdout,
		[]File{},
	)
	if err != nil {
		return fmt.Errorf("Failed to clone repository <%s>: ", repo.Name, err)
	}

	if len(ref) > 0 {
		err = runForegroundContainer(
			ctx,
			cli,
			"alpine/git",
			[]string{"fetch", "--all"},
			[]string{},
			"",
			[]string{},
			WorkingDirectory,
			"",
			VolumeName,
			bindMounts,
			false,
			os.Stdout,
			[]File{},
		)
		if err != nil {
			return fmt.Errorf("Failed to fetch from repository <%s>: %s", repo.Name, err)
		}

		err = runForegroundContainer(
			ctx,
			cli,
			"alpine/git",
			[]string{"checkout", ref},
			[]string{},
			"",
			[]string{},
			WorkingDirectory,
			"",
			VolumeName,
			bindMounts,
			false,
			os.Stdout,
			[]File{},
		)
		if err != nil {
			return fmt.Errorf("Failed to checkout in repository <%s>: %s", repo.Name, err)
		}
	}

	return
}