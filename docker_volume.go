package main

import (
	"context"
	"github.com/docker/docker/api/types/filters"
	dockervolume "github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
)

func RemoveVolume(ctx *context.Context, cli *client.Client, name string) (err error) {
	var result dockervolume.VolumeListOKBody
	result, err = cli.VolumeList(*ctx, filters.NewArgs())
	if err != nil {
		return Error("Failed to list volumes: %s", err)
	}
	for _, volume := range result.Volumes {
		if volume.Name == name {
			err = cli.VolumeRemove(*ctx, volume.Name, false)
			if err != nil {
				return Error("Failed to remove volume with name <%s>: %s", name, err)
			}
		}
	}
	return
}

func CreateVolume(ctx *context.Context, cli *client.Client, name string, driverName string) (err error) {
	_, err = cli.VolumeCreate(*ctx, dockervolume.VolumeCreateBody{
		Name:   name,
		Driver: driverName,
	})
	if err != nil {
		err = Error("Failed to create volume: %s", err)
	}
	return
}
