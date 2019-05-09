package main

import (
	"archive/tar"
	"bytes"
	"context"
	"github.com/docker/cli/cli/command"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/system"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// InjectFile copies the contents of a single file into the volume
func InjectFile(ctx *context.Context, cli *client.Client, id string, srcPath string, dstPath string) (err error) {
	pos := strings.LastIndex(srcPath, "/")
	if pos > -1 {
		dstPath = dstPath + "/" + srcPath[0:pos]
	}

	var absPath string
	absPath, err = filepath.Abs(dstPath)
	if err != nil {
		return Error("Failed to obtain absolute path for path <%s> (source <%s>): %s", dstPath, srcPath, err)
	}

	var dstInfo archive.CopyInfo
	var dstStat types.ContainerPathStat
	dstPath = archive.PreserveTrailingDotOrSeparator(absPath, dstPath, filepath.Separator)
	dstInfo = archive.CopyInfo{Path: dstPath}

	dstStat, err = cli.ContainerStatPath(*ctx, id, dstPath)
	if err != nil {
		return Error("Failed to stat destination path <%s> (source <%s>): %s", dstPath, srcPath, err)
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
		return Error("Destination <%s> must be a directory or regular file", dstPath)
	}
	dstInfo.Exists, dstInfo.IsDir = true, dstStat.Mode.IsDir()

	var srcInfo archive.CopyInfo
	srcInfo, err = archive.CopyInfoSourcePath(srcPath, true)
	if err != nil {
		return Error("Failed to get source info for path <%s>: %s", srcPath, err)
	}

	var srcArchive io.ReadCloser
	srcArchive, err = archive.TarResource(srcInfo)
	if err != nil {
		return Error("Failed to create tar resource for path <%s>: %s", srcPath, err)
	}
	defer srcArchive.Close()

	var dstDir string
	var content io.ReadCloser
	dstDir, content, err = archive.PrepareArchiveCopy(srcArchive, srcInfo, dstInfo)
	if err != nil {
		return Error("Failed to prepare archive reader for path <%s>: %s", srcPath, err)
	}
	defer content.Close()

	err = cli.CopyToContainer(*ctx, id, dstDir, content, types.CopyToContainerOptions{
		AllowOverwriteDirWithFile: false,
	})
	if err != nil {
		return Error("Failed to copy to container for path <%s> (source <%s>): %s", dstDir, srcPath, err)
	}

	return
}

// CreateFile creates a new file with the given content in the volume
func CreateFile(ctx *context.Context, cli *client.Client, id string, name string, data string, dir string) (err error) {
	var content io.ReadCloser
	var dataBytes []byte

	content, writer := io.Pipe()
	dataBytes, err = ioutil.ReadAll(bytes.NewBufferString(data))
	if err != nil {
		return Error("Failed to convert content to bytes for file <%s>: %s", name, err)
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
		return Error("Failed to copy to container for path <%s>: %s", dir, err)
	}

	return
}

// CopyFilesToContainer enumerates all files to copy into the volume
func CopyFilesToContainer(ctx *context.Context, cli *client.Client, id string, files []File, destination string) (err error) {
	for _, file := range files {
		if len(file.Inject) > 0 {
			if len(file.Content) == 0 {
				var matches []string
				matches, err = filepath.Glob(file.Inject)
				if err != nil {
					err = Error("Unable to glob file <%s>", file.Inject)
					return
				}
				if len(matches) == 0 {
					err = Error("No file matches glob <%s>", file.Inject)
					return
				}

				for _, match := range matches {
					Log.Debugf("Injecting file <%s>", match)
					err = InjectFile(ctx, cli, id, match, destination)
					if err != nil {
						err = Error("Failed to inject file <%s>: %s", match, err)
						return
					}
				}

			} else {
				Log.Debugf("Creating file <%s>", file.Inject)
				err = CreateFile(ctx, cli, id, file.Inject, file.Content, destination)
				if err != nil {
					err = Error("Failed to create file <%s>: %s", file.Inject, err)
					return
				}
			}
		}
	}

	return
}

// CopyFilesFromContainer enumerates all files to be copied from the volume
func CopyFilesFromContainer(ctx *context.Context, cli *client.Client, id string, files []File, dir string) (err error) {
	for _, file := range files {
		if len(file.Extract) > 0 {
			srcPath := dir + "/" + file.Extract
			dstPath := file.Destination

			var absPath string
			absPath, err = filepath.Abs(dstPath)
			if err != nil {
				err = Error("Failed to obtain absolute path for path <%s> (source <%s>): %s", dstPath, srcPath, err)
				return
			}
			dstPath = archive.PreserveTrailingDotOrSeparator(absPath, dstPath, filepath.Separator)

			err = command.ValidateOutputPath(dstPath)
			if err != nil {
				err = Error("Failed to validate path <%s>: %s", dstPath, err)
				return
			}

			// if client requests to follow symbol link, then must decide target file to be copied
			var rebaseName string
			var srcStat types.ContainerPathStat
			srcStat, err = cli.ContainerStatPath(*ctx, id, srcPath)
			if err != nil {
				err = Error("Failed to stat destination path <%s> (source <%s>): %s", dstPath, srcPath, err)
				return
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
				err = Error("Failed to copy from container from path <%s>: %s", srcPath, err)
				return
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
				err = Error("Failed to write to disk for path <%s>: %s", dstPath, err)
				return
			}
		}
	}

	return
}
