package main

import (
	"context"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

func removeNetwork(ctx *context.Context, cli *client.Client, name string) (err error) {
	var networks []types.NetworkResource
	networks, err = cli.NetworkList(*ctx, types.NetworkListOptions{
		Filters: filters.NewArgs(),
	})
	if err != nil {
		return Error("Failed to list networks: %s", err)
	}
	for _, network := range networks {
		if network.Name == name {
			err = cli.NetworkRemove(*ctx, network.Name)
			if err != nil {
				return Error("Failed to remove network with name <%s>: %s", name, err)
			}
		}
	}
	return
}

func createNetwork(ctx *context.Context, cli *client.Client, name string, driverName string) (id string, err error) {
	var network types.NetworkCreateResponse
	network, err = cli.NetworkCreate(*ctx, name, types.NetworkCreate{
		Driver: driverName,
	})
	if err != nil {
		err = Error("Failed to create network: %s", err)
		return
	}
	id = network.ID

	return
}