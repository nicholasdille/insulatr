package main

import (
	"context"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"os"
	"strings"
)

func RunStep(ctx *context.Context, cli *client.Client, step Step, globalEnvironment []string) (err error) {
	environment := step.Environment
	for _, globalEnvVar := range globalEnvironment {
		environment = append(environment, globalEnvVar)
	}
	for index, envVarDef := range environment {
		if !strings.Contains(envVarDef, "=") {
			FoundMatch := false
			for _, envVar := range os.Environ() {
				pair := strings.Split(envVar, "=")
				if pair[0] == envVarDef {
					environment[index] = envVar
					FoundMatch = true
				}
			}
			if !FoundMatch {
				err = Error("Unable to find match for environment variable <%s> in build step <%s>", envVarDef, step.Name)
				return
			}
		}
	}

	bindMounts := []mount.Mount{}
	if step.MountDockerSock {
		Log.Warning("Warning: Mounting Docker socket.")
		bindMounts = append(bindMounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: "/var/run/docker.sock",
			Target: "/var/run/docker.sock",
		})
	}
	if step.ForwardSSHAgent {
		err = MapSSHAgentSocket(&environment, &bindMounts)
		if err != nil {
			err = Error("Unable to map SSH agent socket in step <%s>", step.Name)
			return
		}
	}

	err = RunForegroundContainer(
		ctx,
		cli,
		step.Image,
		step.Shell,
		step.Commands,
		step.User,
		environment,
		step.WorkingDirectory,
		step.NetworkName,
		step.VolumeName,
		bindMounts,
		step.OverrideEntrypoint,
		os.Stdout,
		[]File{},
	)
	if err != nil {
		err = Error("Failed to run container: %s", err)
		return
	}

	return
}
