package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/docker/cli/cli/command"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	dockernetwork "github.com/docker/docker/api/types/network"
	dockervolume "github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/system"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func createDockerClient(ctx *context.Context) (*client.Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}
	cli.NegotiateAPIVersion(*ctx)
	return cli, nil
}

func removeVolume(ctx *context.Context, cli *client.Client, name string) (err error) {
	var result dockervolume.VolumeListOKBody
	result, err = cli.VolumeList(*ctx, filters.NewArgs())
	if err != nil {
		return
	}
	for _, volume := range result.Volumes {
		if volume.Name == name {
			err = cli.VolumeRemove(*ctx, volume.Name, false)
			if err != nil {
				return
			}
		}
	}
	return nil
}

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

func createVolume(ctx *context.Context, cli *client.Client, name string, driverName string) (err error) {
	_, err = cli.VolumeCreate(*ctx, dockervolume.VolumeCreateBody{
		Name:   name,
		Driver: driverName,
	})

	return
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

func injectFile(ctx *context.Context, cli *client.Client, id string, srcPath string, dstPath string) (err error) {
	pos := strings.LastIndex(srcPath, "/")
	if pos > -1 {
		dstPath = dstPath + "/" + srcPath[0:pos]
	}

	var absPath string
	absPath, err = filepath.Abs(dstPath)
	if err != nil {
		return fmt.Errorf("Failed to obtain absolute path for path <%s> (source <%s>): %s", dstPath, srcPath, err)
	}

	var dstInfo archive.CopyInfo
	var dstStat types.ContainerPathStat
	dstPath = archive.PreserveTrailingDotOrSeparator(absPath, dstPath, filepath.Separator)
	dstInfo = archive.CopyInfo{Path: dstPath}

	dstStat, err = cli.ContainerStatPath(*ctx, id, dstPath)
	if err != nil {
		return fmt.Errorf("Failed to stat destination path <%s> (source <%s>): %s", dstPath, srcPath, err)
	}
	if dstStat.Mode&os.ModeSymlink != 0 {
		linkTarget := dstStat.LinkTarget
		if !system.IsAbs(linkTarget) {
			dstParent, _ := archive.SplitPathDirEntry(dstPath)
			linkTarget = filepath.Join(dstParent, linkTarget)
		}

		dstInfo.Path = linkTarget
		dstStat, err = cli.ContainerStatPath(*ctx, id, linkTarget)
	}

	err = command.ValidateOutputPathFileMode(dstStat.Mode)
	if err != nil {
		return fmt.Errorf("Destination <%s> must be a directory or regular file", dstPath)
	}
	dstInfo.Exists, dstInfo.IsDir = true, dstStat.Mode.IsDir()

	var srcInfo archive.CopyInfo
	srcInfo, err = archive.CopyInfoSourcePath(srcPath, true)
	if err != nil {
		return fmt.Errorf("Failed to get source info for path <%s>: %s", srcPath, err)
	}

	var srcArchive io.ReadCloser
	srcArchive, err = archive.TarResource(srcInfo)
	if err != nil {
		return fmt.Errorf("Failed to create tar resource for path <%s>: %s", srcPath, err)
	}
	defer srcArchive.Close()

	var dstDir string
	var content io.ReadCloser
	dstDir, content, err = archive.PrepareArchiveCopy(srcArchive, srcInfo, dstInfo)
	if err != nil {
		return fmt.Errorf("Failed to prepare archive reader for path <%s>: %s", srcPath, err)
	}
	defer content.Close()

	err = cli.CopyToContainer(*ctx, id, dstDir, content, types.CopyToContainerOptions{
		AllowOverwriteDirWithFile: false,
	})
	if err != nil {
		return fmt.Errorf("Failed to copy to container for path <%s> (source <%s>): %s", dstDir, srcPath, err)
	}

	return
}

func createFile(ctx *context.Context, cli *client.Client, id string, name string, data string, dir string) (err error) {
	var content io.ReadCloser
	var dataBytes []byte

	content, writer := io.Pipe()
	dataBytes, err = ioutil.ReadAll(bytes.NewBufferString(data))
	if err != nil {
		return fmt.Errorf("Failed to convert content to bytes for file <%s>: %s", name, err)
	}
	t := tar.NewWriter(writer)
	go func() {
		t.WriteHeader(
			&tar.Header{
				Name:    name,
				Mode:    0600,
				Size:    int64(len(dataBytes)),
				ModTime: time.Now(),
			},
		)
		t.Write(dataBytes)
		t.Close()
		writer.Close()
	}()

	err = cli.CopyToContainer(*ctx, id, dir, content, types.CopyToContainerOptions{
		AllowOverwriteDirWithFile: false,
	})
	if err != nil {
		return fmt.Errorf("Failed to copy to container for path <%s>: %s", dir, err)
	}

	return
}

func copyFilesToContainer(ctx *context.Context, cli *client.Client, id string, files []File, destination string) (err error) {
	for _, file := range files {
		if len(file.Content) == 0 {
			var matches []string
			matches, err = filepath.Glob(file.Inject)
			if err != nil {
				err = fmt.Errorf("Unable to glob file <%s>", file.Inject)
				return
			}
			if len(matches) == 0 {
				err = fmt.Errorf("No file matches glob <%s>", file.Inject)
				return
			}

			for _, match := range matches {
				err = injectFile(ctx, cli, id, match, destination)
				if err != nil {
					return fmt.Errorf("Failed to inject file <%s>: %s", match, err)
				}
			}

		} else {
			err = createFile(ctx, cli, id, file.Inject, file.Content, destination)
			if err != nil {
				return fmt.Errorf("Failed to create file <%s>: %s", file.Inject, err)
			}
		}
	}

	return
}

func copyFilesFromContainer(ctx *context.Context, cli *client.Client, id string, files []File, dir string) (err error) {
	for _, file := range files {
		if len(file.Extract) > 0 {
			srcPath := dir + "/" + file.Extract
			dstPath := file.Destination

			var absPath string
			absPath, err = filepath.Abs(dstPath)
			if err != nil {
				return fmt.Errorf("Failed to obtain absolute path for path <%s> (source <%s>): %s", dstPath, srcPath, err)
			}
			dstPath = archive.PreserveTrailingDotOrSeparator(absPath, dstPath, filepath.Separator)

			err = command.ValidateOutputPath(dstPath)
			if err != nil {
				return fmt.Errorf("Failed to validate path <%s>: %s", dstPath, err)
			}

			// if client requests to follow symbol link, then must decide target file to be copied
			var rebaseName string
			var srcStat types.ContainerPathStat
			srcStat, err = cli.ContainerStatPath(*ctx, id, srcPath)
			if err != nil {
				return fmt.Errorf("Failed to stat destination path <%s> (source <%s>): %s", dstPath, srcPath, err)
			}
			if srcStat.Mode&os.ModeSymlink != 0 {
				linkTarget := srcStat.LinkTarget
				if !system.IsAbs(linkTarget) {
					// Join with the parent directory.
					srcParent, _ := archive.SplitPathDirEntry(srcPath)
					linkTarget = filepath.Join(srcParent, linkTarget)
				}

				linkTarget, rebaseName = archive.GetRebaseName(srcPath, linkTarget)
				srcPath = linkTarget
			}

			var content io.ReadCloser
			var stat types.ContainerPathStat
			content, stat, err = cli.CopyFromContainer(*ctx, id, srcPath)
			if err != nil {
				return fmt.Errorf("Failed to copy from container from path <%s>: %s", srcPath, err)
			}
			defer content.Close()

			srcInfo := archive.CopyInfo{
				Path:       srcPath,
				Exists:     true,
				IsDir:      stat.Mode.IsDir(),
				RebaseName: rebaseName,
			}

			preArchive := content
			if len(srcInfo.RebaseName) != 0 {
				_, srcBase := archive.SplitPathDirEntry(srcInfo.Path)
				preArchive = archive.RebaseArchiveEntries(content, srcBase, srcInfo.RebaseName)
			}
			err = archive.CopyTo(preArchive, srcInfo, dstPath)
			if err != nil {
				return fmt.Errorf("Failed to write to disk for path <%s>: %s", dstPath, err)
			}
		}
	}

	return
}

func runForegroundContainer(ctx *context.Context, cli *client.Client, image string, shell []string, commands []string, user string, environment []string, dir string, network string, volume string, binds []mount.Mount, overrideEntrypoint bool, logWriter io.Writer, files []File) (err error) {
	Failed := false

	// pull image
	_, err = cli.ImagePull(*ctx, image, types.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("Failed to pull image <%s>: %s", image, err)
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
	}

	// Extract files
	err = copyFilesFromContainer(ctx, cli, ContainerID, files, dir)
	if err != nil {
		err = fmt.Errorf("Failed to extract files: %s", err)
		Failed = true
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
	_, err = cli.ImagePull(*ctx, image, types.ImagePullOptions{})
	if err != nil {
		return "", fmt.Errorf("Failed to pull image <%s>: %s", image, err)
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
