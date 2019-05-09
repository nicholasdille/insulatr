package main

import (
	"context"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"os"
)

// CloneRepo clones a list of repositories into the volume
func CloneRepo(ctx *context.Context, cli *client.Client, repo Repository) (err error) {
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
		log.Warning("Ignoring shallow because branch was specified.")
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
	err = MapSSHAgentSocket(&environment, &bindMounts)
	if err != nil {
		err = Error("Unable to map SSH agent socket for repo <%s>", repo.Name)
		return
	}

	err = RunForegroundContainer(
		ctx,
		cli,
		"alpine/git",
		commands,
		[]string{},
		"",
		environment,
		repo.WorkingDirectory,
		"",
		repo.VolumeName,
		bindMounts,
		false,
		os.Stdout,
		[]File{},
	)
	if err != nil {
		err = Error("Failed to clone repository <%s>: ", repo.Name, err)
		return
	}

	if len(ref) > 0 {
		err = RunForegroundContainer(
			ctx,
			cli,
			"alpine/git",
			[]string{"fetch", "--all"},
			[]string{},
			"",
			[]string{},
			repo.WorkingDirectory,
			"",
			repo.VolumeName,
			bindMounts,
			false,
			os.Stdout,
			[]File{},
		)
		if err != nil {
			err = Error("Failed to fetch from repository <%s>: %s", repo.Name, err)
			return
		}

		err = RunForegroundContainer(
			ctx,
			cli,
			"alpine/git",
			[]string{"checkout", ref},
			[]string{},
			"",
			[]string{},
			repo.WorkingDirectory,
			"",
			repo.VolumeName,
			bindMounts,
			false,
			os.Stdout,
			[]File{},
		)
		if err != nil {
			err = Error("Failed to checkout in repository <%s>: %s", repo.Name, err)
			return
		}
	}

	return
}
