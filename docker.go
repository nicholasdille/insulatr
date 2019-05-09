package main

import (
	"context"
	"github.com/docker/docker/client"
)

// CreateDockerClient instantiates a new Docker client to talk to the Docker Engine API
func CreateDockerClient(ctx *context.Context) (cli *client.Client, err error) {
	cli, err = client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		err = Error("Failed to create Docker client: %s", err)
		return
	}
	cli.NegotiateAPIVersion(*ctx)

	return cli, nil
}
