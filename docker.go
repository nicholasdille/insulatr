package main

import (
	"bytes"
	"context"
	"encoding/binary"
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

func runForegroundContainer(ctx *context.Context, cli *client.Client, image string, shell []string, commands []string, user string, environment []string, dir string, network string, volume string, overrideEntrypoint bool, mountDockerSock bool, logWriter io.Writer) error {
	// pull image
	_, err := cli.ImagePull(*ctx, image, types.ImagePullOptions{})
	if err != nil {
		return err
	}

	// create container
	containerConfig := container.Config{
		Image:       image,
		AttachStdin: true,
		OpenStdin:   true,
		StdinOnce:   true,
		WorkingDir:  dir,
		Env:         environment,
	}
	if overrideEntrypoint {
		containerConfig.Entrypoint = shell
	} else {
		containerConfig.Cmd = shell
	}
	if len(user) > 0 {
		containerConfig.User = user
	}
	mounts := []mount.Mount{
		{
			Type:   mount.TypeVolume,
			Source: volume,
			Target: dir,
		},
	}
	if mountDockerSock {
		fmt.Printf("Warning: Mounting Docker socket.\n")
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: "/var/run/docker.sock",
			Target: "/var/run/docker.sock",
		})
	}
	endpoints := make(map[string]*dockernetwork.EndpointSettings, 1)
	if len(network) > 0 {
		endpoints[network] = &dockernetwork.EndpointSettings{}
	}
	resp, err := cli.ContainerCreate(
		*ctx,
		&containerConfig,
		&container.HostConfig{
			Mounts: mounts,
		},
		&dockernetwork.NetworkingConfig{
			EndpointsConfig: endpoints,
		},
		"",
	)
	if err != nil {
		return err
	}
	ContainerID := resp.ID

	// Attach
	AttachResp, err := cli.ContainerAttach(*ctx, ContainerID, types.ContainerAttachOptions{
		Stream: true,
		Stdin:  true,
	})
	if err != nil {
		return err
	}
	defer AttachResp.Close()

	// Start container
	if err := cli.ContainerStart(*ctx, ContainerID, types.ContainerStartOptions{}); err != nil {
		return err
	}

	// Send commands
	_, err = io.Copy(AttachResp.Conn, bytes.NewBufferString(strings.Join(commands, "\n")))
	AttachResp.CloseWrite()
	if err != nil {
		return err
	}

	// Retrieve output
	reader, err := cli.ContainerLogs(*ctx, ContainerID, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
	})
	go func() {
		hdr := make([]byte, 8)
		for {
			_, err := reader.Read(hdr)
			if err != nil {
				return
			}
			count := binary.BigEndian.Uint32(hdr[4:])
			dat := make([]byte, count)
			_, err = reader.Read(dat)
			logWriter.Write(dat)
		}
	}()

	// Wait
	statusCh, errCh := cli.ContainerWait(*ctx, ContainerID, container.WaitConditionNotRunning)
	var status container.ContainerWaitOKBody
	select {
	// Waits for timeout
	case <-(*ctx).Done():
		return (*ctx).Err()
		// Waits for error
	case err := <-errCh:
		if err != nil {
			return err
		}
	// Waits for status code
	case status = <-statusCh:
	}

	// Remove container
	err = cli.ContainerRemove(*ctx, ContainerID, types.ContainerRemoveOptions{})
	if err != nil {
		return err
	}

	// Check return code
	if status.StatusCode > 0 {
		return errors.New("Return code not zero (" + strconv.FormatInt(status.StatusCode, 10) + ")")
	}

	return nil
}

func runBackgroundContainer(ctx *context.Context, cli *client.Client, image string, environment []string, network string, name string, privileged bool) (string, error) {
	// pull image
	_, err := cli.ImagePull(*ctx, image, types.ImagePullOptions{})
	if err != nil {
		return "", err
	}

	// create container
	hostConfig := container.HostConfig{}
	if privileged {
		fmt.Printf("Warning: Running privileged container.\n")
		hostConfig.Privileged = true
	}
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
		&hostConfig,
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
	if err := cli.ContainerStart(*ctx, ContainerID, types.ContainerStartOptions{}); err != nil {
		return ContainerID, err
	}

	return ContainerID, err
}

func stopAndRemoveContainer(ctx *context.Context, cli *client.Client, containerID string, logWriter io.Writer) error {
	err := cli.ContainerStop(*ctx, containerID, nil)
	if err != nil {
		return err
	}

	reader, err := cli.ContainerLogs(*ctx, containerID, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return err
	}
	b, err := ioutil.ReadAll(reader)
	if err != nil {
		return err
	}
	logWriter.Write(b)

	hdr := make([]byte, 8)
	for {
		_, err := reader.Read(hdr)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		count := binary.BigEndian.Uint32(hdr[4:])
		dat := make([]byte, count)
		_, err = reader.Read(dat)
		logWriter.Write(dat)
	}

	err = cli.ContainerRemove(*ctx, containerID, types.ContainerRemoveOptions{})
	if err != nil {
		return err
	}

	return nil
}
