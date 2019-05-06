package main

import (
	"context"
	"fmt"
	"github.com/docker/docker/client"
	"io"
	"os"
	"strings"
)

func startService(ctx *context.Context, cli *client.Client, service Service, NetworkName string, build *Build) (id string, err error) {
	for index, envVarDef := range service.Environment {
		if !strings.Contains(envVarDef, "=") {
			FoundMatch := false
			for _, envVar := range build.Environment {
				pair := strings.Split(envVar, "=")
				if pair[0] == envVarDef {
					service.Environment[index] = envVar
					FoundMatch = true
				}
			}
			if !FoundMatch {
				err = fmt.Errorf("Unable to find match for environment variable <%s> in service <%s>", envVarDef, service.Name)
				return
			}
		}
	}

	id, err = runBackgroundContainer(
		ctx,
		cli,
		service.Image,
		service.Environment,
		NetworkName,
		service.Name,
		service.Privileged,
	)
	if err != nil {
		err = fmt.Errorf("Failed to start service <%s>: %s", service.Name, err)
		return
	}

	return
}

func stopService(ctx *context.Context, cli *client.Client, name string, id string, services []Service) (err error) {
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
	err = stopAndRemoveContainer(ctx, cli, id, logWriter)
	if err != nil {
		err = fmt.Errorf("Failed to stop service <%s> with ID <%s>: %s", name, id, err)
		return
	}

	return
}