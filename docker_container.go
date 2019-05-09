package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
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

// ReadContainerLogs parses the container logs provided by the Docker Engine
func ReadContainerLogs(reader io.Reader, logWriter io.Writer) (err error) {
	header := make([]byte, 8)
	for {
		_, err := reader.Read(header)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return Error("Failed to reader log header: %s", err)
		}
		count := binary.BigEndian.Uint32(header[4:])
		data := make([]byte, count)
		_, err = reader.Read(data)
		if err != nil {
			return Error("Failed to read log data: %s", err)
		}
		logWriter.Write(data)
	}
}

// MapSSHAgentSocket updates environment variables and bind mounts to map the SSH agent socket into a container
func MapSSHAgentSocket(environment *[]string, mounts *[]mount.Mount) (err error) {
	for _, envVar := range os.Environ() {
		pair := strings.Split(envVar, "=")
		if pair[0] == "SSH_AUTH_SOCK" {
			*environment = append(
				*environment,
				envVar,
			)
			*mounts = append(
				*mounts,
				mount.Mount{
					Type:   mount.TypeBind,
					Source: pair[1],
					Target: pair[1],
				},
			)
			return
		}
	}
	return Error("Unable to environment variable SSH_AUTH_SOCK: %s", "")
}

// RunForegroundContainer runs a container and waits for it to terminate while streaming the logs before removing the container
func RunForegroundContainer(ctx *context.Context, cli *client.Client, image string, shell []string, commands []string, user string, environment []string, dir string, network string, volume string, binds []mount.Mount, overrideEntrypoint bool, logWriter io.Writer, files []File) (err error) {
	failed := false

	// pull image
	var pullReader io.ReadCloser
	pullReader, err = cli.ImagePull(*ctx, image, types.ImagePullOptions{})
	if err != nil {
		err = Error("Failed to pull image <%s>: %s", image, err)
		return
	}
	scanner := bufio.NewScanner(pullReader)
	for scanner.Scan() {
	}
	if err = scanner.Err(); err != nil {
		err = Error("Failed to read pull messages for image <%s>: %s", image, err)
		return
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
		err = Error("Failed to create container: %s", err)
		return
	}
	id := resp.ID

	// Inject files
	err = CopyFilesToContainer(ctx, cli, id, files, dir)
	if err != nil {
		err = Error("Failed to inject files: %s", err)
		failed = true
	}

	// Attach
	var AttachResp types.HijackedResponse
	if !failed {
		AttachResp, err = cli.ContainerAttach(*ctx, id, types.ContainerAttachOptions{
			Stream: true,
			Stdin:  true,
		})
		if err != nil {
			err = Error("Failed to attach to container: %s", err)
			failed = true
		}
		defer AttachResp.Close()
	}

	// Start container
	if !failed {
		if err = cli.ContainerStart(*ctx, id, types.ContainerStartOptions{}); err != nil {
			err = Error("Failed to start container: %s", err)
			failed = true
		}
	}

	// Send commands
	if !failed {
		_, err = io.Copy(AttachResp.Conn, bytes.NewBufferString(strings.Join(commands, "\n")))
		AttachResp.CloseWrite()
		if err != nil {
			err = Error("Failed to send commands to container: %s", err)
			failed = true
		}
	}

	// Retrieve output
	if !failed {
		reader, err := cli.ContainerLogs(*ctx, id, types.ContainerLogsOptions{
			ShowStdout: true,
			ShowStderr: true,
			Follow:     true,
		})
		if err != nil {
			err = Error("Failed to connect to container logs: %s", err)
			failed = true

		} else {
			go ReadContainerLogs(reader, logWriter)
		}
	}

	// Wait
	var status container.ContainerWaitOKBody
	if !failed {
		statusCh, errCh := cli.ContainerWait(*ctx, id, container.WaitConditionNotRunning)
		select {
		// Waits for timeout
		case <-(*ctx).Done():
			err = Error("Request timed out: %s", (*ctx).Err())
			failed = true
		// Waits for error
		case err := <-errCh:
			if err != nil {
				err = Error("Failed to wait for container: %s", err)
				failed = true
			}
		// Waits for status code
		case status = <-statusCh:
		}
	}

	// Check return code
	if status.StatusCode > 0 {
		err = Error("Return code not zero (%s)", strconv.FormatInt(status.StatusCode, 10))
		failed = true
	}

	// Extract files
	if !failed {
		err = CopyFilesFromContainer(ctx, cli, id, files, dir)
		if err != nil {
			err = Error("Failed to extract files: %s", err)
			failed = true
		}
	}

	// Remove container
	err2 := cli.ContainerRemove(*ctx, id, types.ContainerRemoveOptions{})
	if err2 != nil {
		err2 = Error("Error: Failed to remove container for image <%s>", image)

		if !failed {
			err = err2
			failed = true
		}
	}

	return
}

// RunBackgroundContainer runs a container in the background
func RunBackgroundContainer(ctx *context.Context, cli *client.Client, image string, environment []string, network string, name string, privileged bool) (id string, err error) {
	// pull image
	var pullReader io.ReadCloser
	pullReader, err = cli.ImagePull(*ctx, image, types.ImagePullOptions{})
	if err != nil {
		err = Error("Failed to pull image <%s>: %s", image, err)
		return
	}
	scanner := bufio.NewScanner(pullReader)
	for scanner.Scan() {
	}
	if err = scanner.Err(); err != nil {
		err = Error("Failed to read pull messages for image <%s>: %s", image, err)
		return
	}
	pullReader.Close()

	// create container
	hostConfig := container.HostConfig{}
	if privileged {
		log.Warning("Running privileged container.")
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
		err = Error("Failed to create container: %s", err)
		return
	}
	id = resp.ID
	log.Debugf("Container ID: %s", id)

	// Start container
	if err = cli.ContainerStart(*ctx, id, types.ContainerStartOptions{}); err != nil {
		err = Error("Failed to start container: %s", err)
	}

	return
}

// StopAndRemoveContainer stops a container, reads the logs and removes it
func StopAndRemoveContainer(ctx *context.Context, cli *client.Client, id string, logWriter io.Writer) (err error) {
	err = cli.ContainerStop(*ctx, id, nil)
	if err != nil {
		err = Error("Failed to stop container: %s", err)
		return
	}

	Failed := false
	var reader io.ReadCloser
	reader, err = cli.ContainerLogs(*ctx, id, types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		err = Error("Failed to connect to container logs: %s", err)
		Failed = true
	}
	if !Failed && logWriter != nil {
		err = ReadContainerLogs(reader, logWriter)
		if err != nil {
			err = Error("Failed to read container logs: %s", err)
			return
		}
	}

	err2 := cli.ContainerRemove(*ctx, id, types.ContainerRemoveOptions{})
	if err2 != nil {
		err2 = Error("Error: Failed to remove container <%s>", id)

		if !Failed {
			err = err2
			Failed = true
		}
	}

	return nil
}
