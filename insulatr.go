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
	Name            string   `yaml:"name"`
	Image           string   `yaml:"image"`
	Environment     []string `yaml:"environment"`
        Privileged      bool     `yaml:"privileged"`
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
}

// Build is used to import from YaML
type Build struct {
	Settings     Settings     `yaml:"settings"`
	Repositories []Repository `yaml:"repos"`
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

func run(build *Build, mustReuseVolume, mustRemoveVolume, mustReuseNetwork, mustRemoveNetwork bool, allowDockerSock bool, allowPrivileged bool) error {
        if len(build.Repositories) > 1 {
                for _, repo := range build.Repositories {
                        if len(repo.Directory) == 0 || repo.Directory == "." {
                                return errors.New("All repositories require the directory node to be set (<.> is not allowed).")
                        }
                }
        }

	ctx := context.Background()
	ctxTimeout, cancel := context.WithTimeout(ctx, time.Duration(build.Settings.Timeout)*time.Second)
	defer cancel()

	cli, err := createDockerClient(&ctxTimeout)
	if err != nil {
		return err
	}

	FailedBuild := false

	if mustRemoveVolume {
		fmt.Printf("########## Remove volume\n")
		err = removeVolume(&ctxTimeout, cli, build.Settings.VolumeName)
		if err != nil {
			return err
		}
		fmt.Printf("=== Done\n\n")
	}

	if mustRemoveNetwork {
		fmt.Printf("########## Remove volume\n")
		err = removeNetwork(&ctxTimeout, cli, build.Settings.NetworkName)
		if err != nil {
			return err
		}
		fmt.Printf("=== Done\n\n")
	}

	if !mustReuseVolume {
		fmt.Printf("########## Create volume\n")
		newVolumeName, err := createVolume(&ctxTimeout, cli, build.Settings.VolumeName, build.Settings.VolumeDriver)
		if err != nil {
			fmt.Println(err)
			FailedBuild = true
		}
		fmt.Printf("%s\n\n", newVolumeName)
	}

	if !FailedBuild && !mustReuseNetwork {
		fmt.Printf("########## Create network\n")
		newNetworkID, err := createNetwork(&ctxTimeout, cli, build.Settings.NetworkName, build.Settings.NetworkDriver)
		if err != nil {
			fmt.Println(err)
			FailedBuild = true
		}
		fmt.Printf("%s\n\n", newNetworkID)
	}

	services := make(map[string]string)
	if !FailedBuild {
		fmt.Printf("########## Starting services\n")
		for index, service := range build.Services {
			if service.Name == "" {
				fmt.Printf("Error: Service %d is missing a name.\n", index)
				FailedBuild = true
				break
			}

			fmt.Printf("=== Starting service %s\n", service.Name)

			if service.Image == "" {
				fmt.Printf("Error: Service %s is missing an image.\n", service.Name)
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
				fmt.Println(err)
				FailedBuild = true
				break
			}
			services[service.Name] = containerID
		}
		fmt.Printf("\n")
	}

	if !FailedBuild {
		fmt.Printf("########## Cloning repositories\n")
		for index, repo := range build.Repositories {
			if repo.Name == "" {
				fmt.Printf("Error: Repository %d is missing a name.\n", index)
				FailedBuild = true
				break
			}

			fmt.Printf("=== cloning repo %s\n", repo.Name)

			if repo.Location == "" {
				fmt.Printf("Error: Repository %d is missing a location.\n", repo.Name)
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

			cloneOutput, err := runForegroundContainer(
				&ctxTimeout,
				cli,
				"alpine/git",
				commands,
				[]string{},
				"",
				[]string{},
				build.Settings.WorkingDirectory,
				"",
				build.Settings.VolumeName,
				false,
                                false,
			)
			fmt.Printf("%s\n", cloneOutput)
			if err != nil {
				fmt.Println(err)
				FailedBuild = true
			}

			if len(ref) > 0 {
				fetchOutput, err := runForegroundContainer(
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
					false,
                                        false,
				)
				fmt.Printf("%s\n", fetchOutput)
				if err != nil {
					fmt.Println(err)
					FailedBuild = true
				}

				checkoutOutput, err := runForegroundContainer(
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
					false,
                                        false,
				)
				fmt.Printf("%s\n", checkoutOutput)
				if err != nil {
					fmt.Println(err)
					FailedBuild = true
				}
			}
		}
		fmt.Printf("\n")
	}

	if !FailedBuild {
		fmt.Printf("########## Running build steps\n")
		for index, step := range build.Steps {
			if step.Name == "" {
				fmt.Printf("Error: Step %d is missing a name.\n", index)
				FailedBuild = true
				break
			}

			fmt.Printf("=== running step %s\n", step.Name)

			if len(step.Commands) == 0 {
				fmt.Printf("Error: Step %d is missing commands.\n", index)
				FailedBuild = true
				break
			}

			if len(step.Shell) == 0 {
				step.Shell = build.Settings.Shell
			}

			output, err := runForegroundContainer(
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
				step.OverrideEntrypoint,
                                step.MountDockerSock,
			)
			fmt.Printf("%s\n", output)
			if err != nil {
				fmt.Println(err)
				FailedBuild = true
				break
			}
		}
		fmt.Printf("\n")
	}

	fmt.Printf("########## Stopping services\n")
	for name, containerID := range services {
		fmt.Printf("=== stopping service %s\n", name)
		output, err := stopAndRemoveContainer(&ctx, cli, containerID)
		fmt.Printf("%s\n", output)
		if err != nil {
			fmt.Printf("Error stopping service %s with ID %s\n", name, containerID)
			FailedBuild = true
		}
		delete(services, name)
	}
	fmt.Printf("\n")

	if !mustReuseNetwork {
		fmt.Printf("########## Removing network\n")
		err = removeNetwork(&ctx, cli, build.Settings.NetworkName)
		if err != nil {
			return err
		}
		fmt.Printf("=== Done\n\n")
	}

	if !mustReuseVolume {
		fmt.Printf("########## Removing volume\n")
		err = removeVolume(&ctx, cli, build.Settings.VolumeName)
		if err != nil {
			return err
		}
		fmt.Printf("=== Done\n\n")
	}

	if FailedBuild {
		return errors.New("Build failed")
	}

	return nil
}
