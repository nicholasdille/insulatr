package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	dockernetwork "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"io"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
)

func createDockerClient(ctx *context.Context) (*client.Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}
	cli.NegotiateAPIVersion(*ctx)
	return cli, nil
}

func removeVolume(ctx *context.Context, cli *client.Client, name string) error {
	result, err := cli.VolumeList(*ctx, filters.NewArgs())
	if err != nil {
		return err
	}
	for _, volume := range result.Volumes {
		if volume.Name == name {
			err = cli.VolumeRemove(*ctx, volume.Name, false)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func removeNetwork(ctx *context.Context, cli *client.Client, name string) error {
	networks, err := cli.NetworkList(*ctx, types.NetworkListOptions{
		Filters: filters.NewArgs(),
	})
	if err != nil {
		return err
	}
	for _, network := range networks {
		if network.Name == name {
			err = cli.NetworkRemove(*ctx, network.Name)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func createVolume(ctx *context.Context, cli *client.Client, name string, driverName string) (string, error) {
	volume, err := cli.VolumeCreate(*ctx, volume.VolumeCreateBody{
		Name:   name,
		Driver: driverName,
	})
	if err != nil {
		return "", err
	}

	return volume.Name, nil
}

func createNetwork(ctx *context.Context, cli *client.Client, name string, driverName string) (string, error) {
	network, err := cli.NetworkCreate(*ctx, name, types.NetworkCreate{
		Driver: driverName,
	})
	if err != nil {
		return "", err
	}

	return network.ID, nil
}

func runForegroundContainer(ctx *context.Context, cli *client.Client, image string, shell []string, commands []string, user string, environment []string, dir string, network string, volume string, overrideEntrypoint bool) (string, error) {
	result := ""

	// pull image
	fmt.Printf("=== pull\n")
	reader, err := cli.ImagePull(*ctx, image, types.ImagePullOptions{})
	if err != nil {
		return "", err
	}
	io.Copy(os.Stdout, reader)

	// create container
	fmt.Printf("=== create\n")
	containerConfig := container.Config{
		Image:        image,
		Tty:          false,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		OpenStdin:    true,
		StdinOnce:    true,
		WorkingDir:   dir,
		Env:          environment,
	}
	if overrideEntrypoint {
		containerConfig.Entrypoint = shell
	} else {
		containerConfig.Cmd = shell
	}
	if len(user) > 0 {
		containerConfig.User = user
	}
	endpoints := make(map[string]*dockernetwork.EndpointSettings, 1)
	if len(network) > 0 {
		endpoints[network] = &dockernetwork.EndpointSettings{}
	}
	resp, err := cli.ContainerCreate(
		*ctx,
		&containerConfig,
		&container.HostConfig{
			AutoRemove: true,
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeVolume,
					Source: volume,
					Target: dir,
				},
			},
		},
		&dockernetwork.NetworkingConfig{
			EndpointsConfig: endpoints,
		},
		"",
	)
	if err != nil {
		return "", err
	}
	ContainerID := resp.ID
	fmt.Printf("%s\n", ContainerID)

	// Attach
	fmt.Printf("=== attach\n")
	AttachResp, err := cli.ContainerAttach(*ctx, ContainerID, types.ContainerAttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return "", err
	}
	defer AttachResp.Close()

	// Start container
	fmt.Printf("=== start\n")
	if err := cli.ContainerStart(*ctx, ContainerID, types.ContainerStartOptions{}); err != nil {
		return "", err
	}

	attachCh := make(chan error, 2)
	logCh := make(chan string, 1)

	// Retrieve output
	go func() {
		b, err := ioutil.ReadAll(AttachResp.Reader)
		if err != nil {
			attachCh <- err
		}
		logCh <- string(b)
	}()

	// Send commands
	go func() {
		_, err = io.Copy(AttachResp.Conn, bytes.NewBufferString(strings.Join(commands, "\n")))
		AttachResp.CloseWrite()
		if err != nil {
			attachCh <- err
		}
	}()

	// Wait
	statusCh, errCh := cli.ContainerWait(*ctx, ContainerID, container.WaitConditionNotRunning)
	select {
	case <-(*ctx).Done():
		return "", (*ctx).Err()
	case err := <-errCh:
		if err != nil {
			return "", err
		}
	case err = <-attachCh:
		return "", err
	case status := <-statusCh:
		if status.StatusCode > 0 {
			result = <-logCh
			return result, errors.New("Return code not zero (" + strconv.FormatInt(status.StatusCode, 10) + ")")
		}
	}

	result = <-logCh

	fmt.Printf("=== done\n")

	return result, nil
}

func runBackgroundContainer(ctx *context.Context, cli *client.Client, image string, environment []string, network string, name string) (string, error) {
	// pull image
	fmt.Printf("=== pull\n")
	reader, err := cli.ImagePull(*ctx, image, types.ImagePullOptions{})
	if err != nil {
		return "", err
	}
	io.Copy(os.Stdout, reader)

	// create container
	fmt.Printf("=== create\n")
	endpoints := make(map[string]*dockernetwork.EndpointSettings, 1)
	if len(network) > 0 {
		endpoints[network] = &dockernetwork.EndpointSettings{}
	}
	resp, err := cli.ContainerCreate(
		*ctx,
		&container.Config{
			Image: image,
			Env:   environment,
		},
		&container.HostConfig{},
		&dockernetwork.NetworkingConfig{
			EndpointsConfig: endpoints,
		},
		name,
	)
	if err != nil {
		return "", err
	}
	ContainerID := resp.ID
	fmt.Printf("%s\n", ContainerID)

	// Start container
	fmt.Printf("=== start\n")
	if err := cli.ContainerStart(*ctx, ContainerID, types.ContainerStartOptions{}); err != nil {
		return ContainerID, err
	}

	fmt.Printf("=== done\n")

	return ContainerID, err
}

func stopAndRemoveContainer(ctx *context.Context, cli *client.Client, containerID string) (string, error) {
	result := ""

	err := cli.ContainerStop(*ctx, containerID, nil)
	if err != nil {
		return result, err
	}

	reader, err := cli.ContainerLogs(*ctx, containerID, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return result, err
	}
	b, err := ioutil.ReadAll(reader)
	if err != nil {
		return result, err
	}
	result = string(b)

	err = cli.ContainerRemove(*ctx, containerID, types.ContainerRemoveOptions{})
	if err != nil {
		return result, err
	}

	return result, nil
}