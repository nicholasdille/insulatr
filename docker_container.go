package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	dockernetwork "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"io"
	"os"
	"strconv"
	"strings"
)

func runForegroundContainer(ctx *context.Context, cli *client.Client, image string, shell []string, commands []string, user string, environment []string, dir string, network string, volume string, binds []mount.Mount, overrideEntrypoint bool, logWriter io.Writer, files []File) (err error) {
	Failed := false

	// pull image
	var pullReader io.ReadCloser
	pullReader, err = cli.ImagePull(*ctx, image, types.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("Failed to pull image <%s>: %s", image, err)
	}
	scanner := bufio.NewScanner(pullReader)
	for scanner.Scan() {}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("Failed to read pull messages for image <%s>: %s", image, err)
	}
	pullReader.Close()

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
	for _, bind := range binds {
		mounts = append(mounts, bind)
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
		return fmt.Errorf("Failed to create container: %s", err)
	}
	ContainerID := resp.ID

	// Inject files
	err = copyFilesToContainer(ctx, cli, ContainerID, files, dir)
	if err != nil {
		err = fmt.Errorf("Failed to inject files: %s", err)
		Failed = true
	}

	// Attach
	var AttachResp types.HijackedResponse
	if !Failed {
		AttachResp, err = cli.ContainerAttach(*ctx, ContainerID, types.ContainerAttachOptions{
			Stream: true,
			Stdin:  true,
		})
		if err != nil {
			err = fmt.Errorf("Failed to attach to container: %s", err)
			Failed = true
		}
		defer AttachResp.Close()
	}

	// Start container
	if !Failed {
		if err = cli.ContainerStart(*ctx, ContainerID, types.ContainerStartOptions{}); err != nil {
			err = fmt.Errorf("Failed to start container: %s", err)
			Failed = true
		}
	}

	// Send commands
	if !Failed {
		_, err = io.Copy(AttachResp.Conn, bytes.NewBufferString(strings.Join(commands, "\n")))
		AttachResp.CloseWrite()
		if err != nil {
			err = fmt.Errorf("Failed to send commands to container: %s", err)
			Failed = true
		}
	}

	// Retrieve output
	if !Failed {
		reader, err := cli.ContainerLogs(*ctx, ContainerID, types.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
		})
		if err != nil {
			err = fmt.Errorf("Failed to connect to container logs: %s", err)
			Failed = true

		} else {
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
		}
	}

	// Wait
	var status container.ContainerWaitOKBody
	if !Failed {
		statusCh, errCh := cli.ContainerWait(*ctx, ContainerID, container.WaitConditionNotRunning)
		select {
		// Waits for timeout
		case <-(*ctx).Done():
			err = fmt.Errorf("Request timed out: %s", (*ctx).Err())
			Failed = true
		// Waits for error
		case err := <-errCh:
			if err != nil {
				err = fmt.Errorf("Failed to wait for container: %s", err)
				Failed = true
			}
		// Waits for status code
		case status = <-statusCh:
		}
	}

	// Check return code
	if status.StatusCode > 0 {
		err = fmt.Errorf("Return code not zero (%s)", strconv.FormatInt(status.StatusCode, 10))
		Failed = true
	}

	// Extract files
	if !Failed {
		err = copyFilesFromContainer(ctx, cli, ContainerID, files, dir)
		if err != nil {
			err = fmt.Errorf("Failed to extract files: %s", err)
			Failed = true
		}
	}

	// Remove container
	err2 := cli.ContainerRemove(*ctx, ContainerID, types.ContainerRemoveOptions{})
	if err2 != nil {
		err2 = fmt.Errorf("Error: Failed to remove container for image <%s>", image)

		if Failed {
			fmt.Fprintf(os.Stderr, "%s\n", err2)
		} else {
			err = err2
		}
	}

	return
}

func runBackgroundContainer(ctx *context.Context, cli *client.Client, image string, environment []string, network string, name string, privileged bool) (id string, err error) {
	// pull image
	var pullReader io.ReadCloser
	pullReader, err = cli.ImagePull(*ctx, image, types.ImagePullOptions{})
	if err != nil {
		return "", fmt.Errorf("Failed to pull image <%s>: %s", image, err)
	}
	scanner := bufio.NewScanner(pullReader)
	for scanner.Scan() {}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("Failed to read pull messages for image <%s>: %s", image, err)
	}
	pullReader.Close()

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
	var resp container.ContainerCreateCreatedBody
	resp, err = cli.ContainerCreate(
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
		return "", fmt.Errorf("Failed to create container: %s", err)
	}
	id = resp.ID
	fmt.Printf("%s\n", id)

	// Start container
	if err = cli.ContainerStart(*ctx, id, types.ContainerStartOptions{}); err != nil {
		err = fmt.Errorf("Failed to start container: %s", err)
	}

	return
}

func stopAndRemoveContainer(ctx *context.Context, cli *client.Client, containerID string, logWriter io.Writer) (err error) {
	err = cli.ContainerStop(*ctx, containerID, nil)
	if err != nil {
		return fmt.Errorf("Failed to stop container: %s", err)
	}

	var reader io.ReadCloser
	reader, err = cli.ContainerLogs(*ctx, containerID, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		return fmt.Errorf("Failed to connect to container logs: %s", err)
	}
	if logWriter != nil {
		hdr := make([]byte, 8)
		for {
			_, err = reader.Read(hdr)
			if err != nil {
				if err == io.EOF {
					break
				}
				return fmt.Errorf("Failed to read header from container logs: %s", err)
			}
			count := binary.BigEndian.Uint32(hdr[4:])
			dat := make([]byte, count)
			_, err = reader.Read(dat)
			logWriter.Write(dat)
		}
	}

	err = cli.ContainerRemove(*ctx, containerID, types.ContainerRemoveOptions{})
	if err != nil {
		return fmt.Errorf("Error: Failed to remove container <%s>", containerID)
	}

	return nil
}