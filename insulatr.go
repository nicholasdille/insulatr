package main

import (
	"context"
	"errors"
	"fmt"
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

	if !FailedBuild {
		err = expandGlobalEnvironment(build)
		if err != nil {
			err = fmt.Errorf("Failed to expand global environment: %s", err)
			FailedBuild = true
		}
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

			err = cloneRepo(&ctxTimeout, cli, repo, build.Settings.WorkingDirectory, build.Settings.VolumeName)
			if err != nil {
				err = fmt.Errorf("Failed to clone repository <%s>: %s", repo.Name, err)
				FailedBuild = true
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

			var containerID string
			containerID, err = startService(&ctxTimeout, cli, service, build.Settings.NetworkName, build)
			if err != nil {
				err = fmt.Errorf("Failed to start service <%s>: %s", service.Name, err)
				FailedBuild = true
				break
			}
			
			services[service.Name] = containerID
		}
		fmt.Printf("\n")
	}

	if !FailedBuild && len(build.Files) > 0 {
		fmt.Printf("########## Injecting files\n")

		if !FailedBuild {
			err = injectFiles(&ctxTimeout, cli, build.Files, build.Settings.WorkingDirectory, build.Settings.VolumeName)
			if err != nil {
				err = fmt.Errorf("Failed to inject files: %s", err)
				FailedBuild = true
			}
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

			err = runStep(&ctxTimeout, cli, step, build.Environment, build.Settings.Shell, build.Settings.WorkingDirectory, build.Settings.VolumeName, build.Settings.NetworkName)
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

		err = extractFiles(&ctxTimeout, cli, build.Files, build.Settings.WorkingDirectory, build.Settings.VolumeName)
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

			err = stopService(&ctxTimeout, cli, name, containerID, build.Services)
			if err != nil {
				err = fmt.Errorf("Failed to stop service <%s> with container ID <%s>: %s", name, containerID, err)
				FailedBuild = true
				break
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
