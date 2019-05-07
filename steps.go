package main

import (
	"context"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"os"
	"strings"
)

func runStep(ctx *context.Context, cli *client.Client, step Step, globalEnvironment []string, shell []string, WorkingDirectory string, VolumeName string, NetworkName string) (err error) {
	if len(step.Shell) == 0 {
		step.Shell = shell
	}

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
		log.Warning("Warning: Mounting Docker socket.")
		bindMounts = append(bindMounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: "/var/run/docker.sock",
			Target: "/var/run/docker.sock",
		})
	}
	if step.ForwardSSHAgent {
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
	}

	err = runForegroundContainer(
		ctx,
		cli,
		step.Image,
		step.Shell,
		step.Commands,
		step.User,
		environment,
		WorkingDirectory,
		NetworkName,
		VolumeName,
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