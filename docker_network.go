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
		return
	}
	for _, network := range networks {
		if network.Name == name {
			err = cli.NetworkRemove(*ctx, network.Name)
			if err != nil {
				return
			}
		}
	}
	return nil
}

func createNetwork(ctx *context.Context, cli *client.Client, name string, driverName string) (id string, err error) {
	var network types.NetworkCreateResponse
	network, err = cli.NetworkCreate(*ctx, name, types.NetworkCreate{
		Driver: driverName,
	})
	if err != nil {
		return
	}
	id = network.ID

	return
}