package main

import (
	"context"
	"github.com/docker/docker/client"
	"io"
	"os"
	"strings"
)

// StartService starts a single service in a container
func StartService(ctx *context.Context, cli *client.Client, service Service, build *Build) (id string, err error) {
	for index, envVarDef := range service.Environment {
		if !strings.Contains(envVarDef, "=") {
			foundMatch := false
			for _, envVar := range build.Environment {
				pair := strings.Split(envVar, "=")
				if pair[0] == envVarDef {
					service.Environment[index] = envVar
					foundMatch = true
				}
			}
			if !foundMatch {
				err = Error("Unable to find match for environment variable <%s> in service <%s>", envVarDef, service.Name)
				return
			}
		}
	}

	id, err = RunBackgroundContainer(
		ctx,
		cli,
		service.Image,
		service.Environment,
		service.NetworkName,
		service.Name,
		service.Privileged,
	)
	if err != nil {
		err = Error("Failed to start service <%s>: %s", service.Name, err)
		return
	}

	return
}

// StopService stops a single service
func StopService(ctx *context.Context, cli *client.Client, name string, id string, services []Service) (err error) {
	var logWriter io.Writer
	logWriter = os.Stdout
	var service Service
	for _, service = range services {
		if service.Name == name {
			break
		}
	}
	if service.SuppressLog {
		logWriter = nil
	}
	err = StopAndRemoveContainer(ctx, cli, id, logWriter)
	if err != nil {
		err = Error("Failed to stop service <%s> with ID <%s>: %s", name, id, err)
		return
	}

	return
}
