package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/docker/docker/api/types/mount"
	"io"
	"os"
	"strings"
	"time"
)

// Settings is used to import from YaML
type Settings struct {
	VolumeName       string   `yaml:"volume_name"`
	VolumeDriver     string   `yaml:"volume_driver"`
	WorkingDirectory string   `yaml:"working_directory"`
	Shell            []string `yaml:"shell"`
	NetworkName      string   `yaml:"network_name"`
	NetworkDriver    string   `yaml:"network_driver"`
	Timeout          int      `yaml:"timeout"`
}

// Repository is used to import from YaML
type Repository struct {
	Name      string `yaml:"name"`
	Location  string `yaml:"location"`
	Directory string `yaml:"directory"`
	Shallow   bool   `yaml:"shallow"`
	Branch    string `yaml:"branch"`
	Tag       string `yaml:"tag"`
	Commit    string `yaml:"commit"`
}

// Service is used to import from YaML
type Service struct {
	Name        string   `yaml:"name"`
	Image       string   `yaml:"image"`
	Environment []string `yaml:"environment"`
	SuppressLog bool     `yaml:"suppress_log"`
	Privileged  bool     `yaml:"privileged"`
}

// File is used to import from YaML
type File struct {
	Inject      string `yaml:"inject"`
	Content     string `yaml:"content"`
	Extract     string `yaml:"extract"`
	Destination string
}

// Step is used to import from YaML
type Step struct {
	Name               string   `yaml:"name"`
	Image              string   `yaml:"image"`
	Shell              []string `yaml:"shell"`
	OverrideEntrypoint bool     `yaml:"override_entrypoint"`
	User               string   `yaml:"user"`
	Commands           []string `yaml:"commands"`
	Environment        []string `yaml:"environment"`
	MountDockerSock    bool     `yaml:"mount_docker_sock"`
	ForwardSSHAgent    bool     `yaml:"forward_ssh_agent"`
}

// Build is used to import from YaML
type Build struct {
	Settings     Settings     `yaml:"settings"`
	Repositories []Repository `yaml:"repos"`
	Files        []File       `yaml:"files"`
	Services     []Service    `yaml:"services"`
	Environment  []string     `yaml:"environment"`
	Steps        []Step       `yaml:"steps"`
}

func defaults() *Build {
	return &Build{
		Settings: Settings{
			VolumeName:       "myvolume",
			VolumeDriver:     "local",
			WorkingDirectory: "/src",
			Shell:            []string{"sh"},
			Timeout:          60 * 60,
			NetworkName:      "mynetwork",
			NetworkDriver:    "bridge",
		},
	}
}

func run(build *Build, mustReuseVolume, mustRemoveVolume, mustReuseNetwork, mustRemoveNetwork bool, allowDockerSock bool, allowPrivileged bool) (err error) {
	if len(build.Repositories) > 1 {
		for _, repo := range build.Repositories {
			if len(repo.Directory) == 0 || repo.Directory == "." {
				return errors.New("All repositories require the directory node to be set (<.> is not allowed)")
			}
		}
	}

	ctx := context.Background()
	ctxTimeout, cancel := context.WithTimeout(ctx, time.Duration(build.Settings.Timeout)*time.Second)
	defer cancel()

	cli, err := createDockerClient(&ctxTimeout)
	if err != nil {
		return fmt.Errorf("Unable to create Docker client: %s", err)
	}

	FailedBuild := false

	if mustRemoveVolume {
		fmt.Printf("########## Remove volume\n")
		err = removeVolume(&ctxTimeout, cli, build.Settings.VolumeName)
		if err != nil {
			return fmt.Errorf("Failed to remove volume: %s", err)
		}
		fmt.Printf("=== Done\n\n")
	}

	if mustRemoveNetwork {
		fmt.Printf("########## Remove network\n")
		err = removeNetwork(&ctxTimeout, cli, build.Settings.NetworkName)
		if err != nil {
			return fmt.Errorf("Failed to remove network: %s", err)
		}
		fmt.Printf("=== Done\n\n")
	}

	if !mustReuseVolume {
		fmt.Printf("########## Create volume\n")
		err := createVolume(&ctxTimeout, cli, build.Settings.VolumeName, build.Settings.VolumeDriver)
		if err != nil {
			return fmt.Errorf("Failed to create volume: %s", err)
		}
		fmt.Printf("%s\n\n", build.Settings.VolumeName)
	}

	if !FailedBuild && !mustReuseNetwork {
		fmt.Printf("########## Create network\n")
		newNetworkID, err := createNetwork(&ctxTimeout, cli, build.Settings.NetworkName, build.Settings.NetworkDriver)
		if err != nil {
			err = fmt.Errorf("Failed to create network: %s", err)
			FailedBuild = true
		}
		fmt.Printf("%s\n\n", newNetworkID)
	}

	if !FailedBuild && len(build.Files) > 0 {
		fmt.Printf("########## Injecting files\n")

		for index, file := range build.Files {
			if len(file.Inject) == 0 {
				err = fmt.Errorf("Name of injected file must be set for entry at index <%d>", index)
				FailedBuild = true
			}
		}

		if !FailedBuild {
			filesToInject := []File{}
			for _, file := range build.Files {
				if len(file.Inject) > 0 {
					filesToInject = append(filesToInject, file)
				}
			}

			err := runForegroundContainer(
				&ctxTimeout,
				cli,
				"alpine",
				[]string{"sh"},
				[]string{},
				"",
				[]string{},
				build.Settings.WorkingDirectory,
				"",
				build.Settings.VolumeName,
				[]mount.Mount{},
				false,
				os.Stdout,
				filesToInject,
			)
			if err != nil {
				err = fmt.Errorf("Failed to inject files: %s", err)
				FailedBuild = true
			}
		}

		fmt.Printf("\n")
	}

	if !FailedBuild && len(build.Repositories) > 0 {
		fmt.Printf("########## Cloning repositories\n")
		for index, repo := range build.Repositories {
			if repo.Name == "" {
				err = fmt.Errorf("Repository at index <%d> is missing a name", index)
				FailedBuild = true
				break
			}

			fmt.Printf("=== Cloning repository <%s>\n", repo.Name)

			if repo.Location == "" {
				err = fmt.Errorf("Repository at index <%d> is missing a location", repo.Name)
				FailedBuild = true
				break
			}

			var ref string
			if len(repo.Branch) > 0 {
				ref = repo.Branch
			}
			if len(ref) == 0 && len(repo.Tag) > 0 {
				ref = repo.Tag
			}
			if len(ref) == 0 || len(repo.Commit) > 0 {
				ref = repo.Commit
			}
			if len(ref) > 0 {
				fmt.Printf("Ignoring shallow because branch was specified.\n")
				repo.Shallow = false
			}

			commands := []string{"clone"}
			if repo.Shallow {
				commands = append(commands, "--depth", "1")
			}
			commands = append(commands, repo.Location)
			if len(repo.Directory) > 0 {
				commands = append(commands, repo.Directory)
			}

			environment := []string{
				"GIT_SSH_COMMAND=ssh -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no",
			}
			bindMounts := []mount.Mount{}
			for _, envVar := range os.Environ() {
				pair := strings.Split(envVar, "=")
				if pair[0] == "SSH_AUTH_SOCK" {
					environment = append(
						environment,
						envVar,
					)
					bindMounts = append(
						bindMounts,
						mount.Mount{
							Type:   mount.TypeBind,
							Source: pair[1],
							Target: pair[1],
						},
					)
					break
				}
			}
			err := runForegroundContainer(
				&ctxTimeout,
				cli,
				"alpine/git",
				commands,
				[]string{},
				"",
				environment,
				build.Settings.WorkingDirectory,
				"",
				build.Settings.VolumeName,
				bindMounts,
				false,
				os.Stdout,
				[]File{},
			)
			if err != nil {
				err = fmt.Errorf("Failed to clone repository <%s>: ", repo.Name, err)
				FailedBuild = true
				break
			}

			if len(ref) > 0 {
				err := runForegroundContainer(
					&ctxTimeout,
					cli,
					"alpine/git",
					[]string{"fetch", "--all"},
					[]string{},
					"",
					[]string{},
					build.Settings.WorkingDirectory,
					"",
					build.Settings.VolumeName,
					bindMounts,
					false,
					os.Stdout,
					[]File{},
				)
				if err != nil {
					err = fmt.Errorf("Failed to fetch from repository <%s>: %s", repo.Name, err)
					FailedBuild = true
					break
				}

				err = runForegroundContainer(
					&ctxTimeout,
					cli,
					"alpine/git",
					[]string{"checkout", ref},
					[]string{},
					"",
					[]string{},
					build.Settings.WorkingDirectory,
					"",
					build.Settings.VolumeName,
					bindMounts,
					false,
					os.Stdout,
					[]File{},
				)
				if err != nil {
					err = fmt.Errorf("Failed to checkout in repository <%s>: %s", repo.Name, err)
					FailedBuild = true
					break
				}
			}
		}
		fmt.Printf("\n")
	}

	services := make(map[string]string)
	if !FailedBuild && len(build.Services) > 0 {
		fmt.Printf("########## Starting services\n")
		for index, service := range build.Services {
			if service.Name == "" {
				err = fmt.Errorf("Service at index <%d> is missing a name", index)
				FailedBuild = true
				break
			}

			fmt.Printf("=== Starting service <%s>\n", service.Name)

			if service.Image == "" {
				err = fmt.Errorf("Service <%s> is missing an image", service.Name)
				FailedBuild = true
				break
			}

			containerID, err := runBackgroundContainer(
				&ctxTimeout,
				cli,
				service.Image,
				service.Environment,
				build.Settings.NetworkName,
				service.Name,
				service.Privileged,
			)
			if err != nil {
				err = fmt.Errorf("Failed to start service <%s>: %s", service.Name, err)
				FailedBuild = true
				break
			}
			services[service.Name] = containerID
		}
		fmt.Printf("\n")
	}

	if !FailedBuild && len(build.Steps) > 0 {
		fmt.Printf("########## Running build steps\n")
		for index, step := range build.Steps {
			if step.Name == "" {
				err = fmt.Errorf("Step at index <%d> is missing a name", index)
				FailedBuild = true
				break
			}
			if step.Image == "" {
				err = fmt.Errorf("Step at index <%d> is missing an image", index)
				FailedBuild = true
				break
			}

			fmt.Printf("=== running step <%s>\n", step.Name)

			if len(step.Commands) == 0 {
				err = fmt.Errorf("Step <%s> is missing commands", step.Name)
				FailedBuild = true
				break
			}

			if len(step.Shell) == 0 {
				step.Shell = build.Settings.Shell
			}

			for index, envVarDef := range step.Environment {
				if !strings.Contains(envVarDef, "=") {
					FoundMatch := false
					for _, envVar := range os.Environ() {
						pair := strings.Split(envVar, "=")
						if pair[0] == envVarDef {
							step.Environment[index] = envVar
							FoundMatch = true
						}
					}
					if !FoundMatch {
						err = fmt.Errorf("Unable to find match for environment variable <%s>", envVarDef)
						FailedBuild = true
					}
				}
			}
			if FailedBuild {
				break
			}

			environment := []string{}
			bindMounts := []mount.Mount{}
			if step.MountDockerSock {
				fmt.Printf("Warning: Mounting Docker socket.\n")
				bindMounts = append(bindMounts, mount.Mount{
					Type:   mount.TypeBind,
					Source: "/var/run/docker.sock",
					Target: "/var/run/docker.sock",
				})
			}
			if step.ForwardSSHAgent {
				for _, envVar := range os.Environ() {
					pair := strings.Split(envVar, "=")
					if pair[0] == "SSH_AUTH_SOCK" {
						environment = append(
							environment,
							envVar,
						)
						bindMounts = append(
							bindMounts,
							mount.Mount{
								Type:   mount.TypeBind,
								Source: pair[1],
								Target: pair[1],
							},
						)
						//break
					}
				}
			}

			err = runForegroundContainer(
				&ctxTimeout,
				cli,
				step.Image,
				step.Shell,
				step.Commands,
				step.User,
				step.Environment,
				build.Settings.WorkingDirectory,
				build.Settings.NetworkName,
				build.Settings.VolumeName,
				bindMounts,
				step.OverrideEntrypoint,
				os.Stdout,
				[]File{},
			)
			if err != nil {
				err = fmt.Errorf("Failed to run build step <%s>: %s", step.Name, err)
				FailedBuild = true
				break
			}
			fmt.Printf("\n")
		}
	}

	if !FailedBuild && len(build.Files) > 0 {
		fmt.Printf("########## Extracting files\n")

		filesToExtract := []File{}
		for _, file := range build.Files {
			if len(file.Extract) > 0 {
				file.Destination = "."
				filesToExtract = append(filesToExtract, file)
			}
		}

		err := runForegroundContainer(
			&ctxTimeout,
			cli,
			"alpine",
			[]string{"sh"},
			[]string{},
			"",
			[]string{},
			build.Settings.WorkingDirectory,
			"",
			build.Settings.VolumeName,
			[]mount.Mount{},
			false,
			os.Stdout,
			filesToExtract,
		)
		if err != nil {
			err = fmt.Errorf("Failed to extract files: %s", err)
			FailedBuild = true
		}

		fmt.Printf("\n")
	}

	if len(services) > 0 {
		fmt.Printf("########## Stopping services\n")
		for name, containerID := range services {
			fmt.Printf("=== stopping service %s\n", name)
			var logWriter io.Writer
			logWriter = os.Stdout
			var service Service
			for _, service = range build.Services {
				if service.Name == name {
					break
				}
			}
			if service.SuppressLog {
				logWriter = nil
			}
			err := stopAndRemoveContainer(&ctx, cli, containerID, logWriter)
			if err != nil {
				err = fmt.Errorf("Failed to stop service <%s> with ID <%s>: %s", name, containerID, err)
				FailedBuild = true
			}
			delete(services, name)
		}
		fmt.Printf("\n")
	}

	if !mustReuseNetwork {
		fmt.Printf("########## Removing network\n")
		err := removeNetwork(&ctx, cli, build.Settings.NetworkName)
		if err != nil {
			return fmt.Errorf("Failed to remove network: %s", err)
		}
		fmt.Printf("=== Done\n\n")
	}

	if !mustReuseVolume {
		fmt.Printf("########## Removing volume\n")
		err := removeVolume(&ctx, cli, build.Settings.VolumeName)
		if err != nil {
			return fmt.Errorf("Failed to remove volume: %s", err)
		}
		fmt.Printf("=== Done\n\n")
	}

	return
}
