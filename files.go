package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"os"
)

func InjectFiles(ctx *context.Context, cli *client.Client, files []File, WorkingDirectory string, VolumeName string) (err error) {
	filesToInject := []File{}
	for _, file := range files {
		if len(file.Inject) > 0 {
			filesToInject = append(filesToInject, file)
		}
	}

	err = RunForegroundContainer(
		ctx,
		cli,
		"alpine",
		[]string{"sh"},
		[]string{},
		"",
		[]string{},
		WorkingDirectory,
		"",
		VolumeName,
		[]mount.Mount{},
		false,
		os.Stdout,
		filesToInject,
	)
	if err != nil {
		message := fmt.Sprintf("Failed to run container: %s", err)
		Log.Error(message)
		err = errors.New(message)
		return
	}

	return
}

func ExtractFiles(ctx *context.Context, cli *client.Client, files []File, WorkingDirectory string, VolumeName string) (err error) {
	filesToExtract := []File{}
	for _, file := range files {
		if len(file.Extract) > 0 {
			file.Destination = "."
			filesToExtract = append(filesToExtract, file)
		}
	}

	err = RunForegroundContainer(
		ctx,
		cli,
		"alpine",
		[]string{"sh"},
		[]string{},
		"",
		[]string{},
		WorkingDirectory,
		"",
		VolumeName,
		[]mount.Mount{},
		false,
		os.Stdout,
		filesToExtract,
	)
	if err != nil {
		err = Error("Failed to run container: %s", err)
		return
	}

	return
}
