package main

import (
	"context"
	"github.com/docker/docker/client"
)

func createDockerClient(ctx *context.Context) (cli *client.Client, err error) {
	cli, err = client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		err = Error("Failed to create Docker client: %s", err)
		return
	}
	cli.NegotiateAPIVersion(*ctx)

	return cli, nil
}