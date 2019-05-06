package main

import (
	"context"
	"github.com/docker/docker/client"
)

func createDockerClient(ctx *context.Context) (*client.Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}
	cli.NegotiateAPIVersion(*ctx)
	return cli, nil
}