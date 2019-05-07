package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/op/go-logging"
	"io"
	"os"
	"time"
)

// Settings is used to import from YaML
type Settings struct {
	VolumeName       string        `yaml:"volume_name"`
	VolumeDriver     string        `yaml:"volume_driver"`
	WorkingDirectory string        `yaml:"working_directory"`
	Shell            []string      `yaml:"shell"`
	NetworkName      string        `yaml:"network_name"`
	NetworkDriver    string        `yaml:"network_driver"`
	Timeout          int           `yaml:"timeout"`
	LogDirectory     string        `yaml:"log_directory"`
	ConsoleLogLevel  logging.Level `yaml:"console_log_level"`
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
			LogDirectory:     "logs",
			ConsoleLogLevel:  logging.INFO,
		},
	}
}

var log = logging.MustGetLogger("insulatr")
var FileFormat = logging.MustStringFormatter(
    `%{time:2006-01-02T15:04:05.999Z-07:00} %{level:.7s} %{message}`,
)
var ConsoleFormat = logging.MustStringFormatter(
    `%{color}%{time:15:04:05} %{level:.7s} %{message}%{color:reset}`,
)

func prepareLogging(FileWriter io.Writer) {
    FileBackend    := logging.NewLogBackend(FileWriter, "", 0)
	FileBackendFormatter := logging.NewBackendFormatter(FileBackend, FileFormat)
    FileBackendLeveled := logging.AddModuleLevel(FileBackendFormatter)
    FileBackendLeveled.SetLevel(logging.INFO, "")

    ConsoleBackend := logging.NewLogBackend(os.Stdout, "", 0)
	ConsoleBackendFormatter := logging.NewBackendFormatter(ConsoleBackend, ConsoleFormat)
    ConsoleBackendLeveled := logging.AddModuleLevel(ConsoleBackendFormatter)
    ConsoleBackendLeveled.SetLevel(logging.DEBUG, "")
    
	logging.SetBackend(FileBackendLeveled, ConsoleBackendLeveled)
}

func Error(format string, a ...interface{}) (err error) {
	message := fmt.Sprintf(format, a)
	log.Error(message)
	return errors.New(message)
}

func run(build *Build, mustReuseVolume, mustRemoveVolume, mustReuseNetwork, mustRemoveNetwork bool, allowDockerSock bool, allowPrivileged bool) (err error) {
	if _, err := os.Stat(build.Settings.LogDirectory); os.IsNotExist(err) {
		os.Mkdir(build.Settings.LogDirectory, 0755)
	}

	FileWriter, err := os.OpenFile("logs/test.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
    if err != nil {
		return Error("Failed to open file: ", err)
    }
	prepareLogging(FileWriter)
	log.Noticef("Running insulatr version %s built at %s from %s\n", Version, BuildTime, GitCommit)

	if len(build.Repositories) > 1 {
		for _, repo := range build.Repositories {
			if len(repo.Directory) == 0 || repo.Directory == "." {
				return Error("All repositories require the directory node to be set (<.> is not allowed)")
			}
		}
	}

	ctx := context.Background()
	ctxTimeout, cancel := context.WithTimeout(ctx, time.Duration(build.Settings.Timeout)*time.Second)
	defer cancel()

	cli, err := createDockerClient(&ctxTimeout)
	if err != nil {
		return Error("Unable to create Docker client: %s", err)
	}

	FailedBuild := false

	if mustRemoveVolume {
		log.Debug("########## Remove volume")
		err = removeVolume(&ctxTimeout, cli, build.Settings.VolumeName)
		if err != nil {
			return Error("Failed to remove volume: %s", err)
		}
	}

	if mustRemoveNetwork {
		log.Debug("########## Remove network")
		err = removeNetwork(&ctxTimeout, cli, build.Settings.NetworkName)
		if err != nil {
			return Error("Failed to remove network: %s", err)
		}
	}

	if !mustReuseVolume {
		log.Debug("########## Create volume")
		err := createVolume(&ctxTimeout, cli, build.Settings.VolumeName, build.Settings.VolumeDriver)
		if err != nil {
			return Error("Failed to create volume: %s", err)
		}
		log.Debugf("Volume name: %s", build.Settings.VolumeName)
	}

	if !FailedBuild && !mustReuseNetwork {
		log.Debug("########## Create network")
		newNetworkID, err := createNetwork(&ctxTimeout, cli, build.Settings.NetworkName, build.Settings.NetworkDriver)
		if err != nil {
			err = Error("Failed to create network: %s", err)
			FailedBuild = true
		}
		log.Debugf("Network ID: %s", newNetworkID)
	}

	if !FailedBuild {
		err = expandGlobalEnvironment(build)
		if err != nil {
			err = Error("Failed to expand global environment: %s", err)
			FailedBuild = true
		}
	}

	if !FailedBuild && len(build.Repositories) > 0 {
		log.Notice("########## Cloning repositories")
		for index, repo := range build.Repositories {
			if repo.Name == "" {
				err = Error("Repository at index <%d> is missing a name", index)
				FailedBuild = true
				break
			}

			log.Noticef("########## Cloning repository <%s>", repo.Name)

			if repo.Location == "" {
				err = Error("Repository at index <%d> is missing a location", repo.Name)
				FailedBuild = true
				break
			}

			err = cloneRepo(&ctxTimeout, cli, repo, build.Settings.WorkingDirectory, build.Settings.VolumeName)
			if err != nil {
				err = Error("Failed to clone repository <%s>: %s", repo.Name, err)
				FailedBuild = true
			}
		}
	}

	services := make(map[string]string)
	if !FailedBuild && len(build.Services) > 0 {
		log.Notice("########## Starting services")
		for index, service := range build.Services {
			if service.Name == "" {
				err = Error("Service at index <%d> is missing a name", index)
				FailedBuild = true
				break
			}

			log.Noticef("########## Starting service <%s>", service.Name)

			if service.Image == "" {
				err = Error("Service <%s> is missing an image", service.Name)
				FailedBuild = true
				break
			}

			var containerID string
			containerID, err = startService(&ctxTimeout, cli, service, build.Settings.NetworkName, build)
			if err != nil {
				err = Error("Failed to start service <%s>: %s", service.Name, err)
				FailedBuild = true
				break
			}
			
			services[service.Name] = containerID
		}
	}

	if !FailedBuild && len(build.Files) > 0 {
		log.Notice("########## Injecting files")

		if !FailedBuild {
			err = injectFiles(&ctxTimeout, cli, build.Files, build.Settings.WorkingDirectory, build.Settings.VolumeName)
			if err != nil {
				err = Error("Failed to inject files: %s", err)
				FailedBuild = true
			}
		}
	}

	if !FailedBuild && len(build.Steps) > 0 {
		log.Notice("########## Running build steps")
		for index, step := range build.Steps {
			if step.Name == "" {
				err = Error("Step at index <%d> is missing a name", index)
				FailedBuild = true
				break
			}
			if step.Image == "" {
				err = Error("Step at index <%d> is missing an image", index)
				FailedBuild = true
				break
			}

			log.Noticef("########## running step <%s>", step.Name)

			if len(step.Commands) == 0 {
				err = Error("Step <%s> is missing commands", step.Name)
				FailedBuild = true
				break
			}

			err = runStep(&ctxTimeout, cli, step, build.Environment, build.Settings.Shell, build.Settings.WorkingDirectory, build.Settings.VolumeName, build.Settings.NetworkName)
			if err != nil {
				err = Error("Failed to run build step <%s>: %s", step.Name, err)
				FailedBuild = true
				break
			}
		}
	}

	if !FailedBuild && len(build.Files) > 0 {
		log.Notice("########## Extracting files")

		err = extractFiles(&ctxTimeout, cli, build.Files, build.Settings.WorkingDirectory, build.Settings.VolumeName)
		if err != nil {
			err = Error("Failed to extract files: %s", err)
			FailedBuild = true
		}
	}

	if len(services) > 0 {
		for name, containerID := range services {
			log.Noticef("########## Stopping service %s", name)

			err = stopService(&ctxTimeout, cli, name, containerID, build.Services)
			if err != nil {
				err = Error("Failed to stop service <%s> with container ID <%s>: %s", name, containerID, err)
				FailedBuild = true
				break
			}

			delete(services, name)
		}
	}

	if !mustReuseNetwork {
		log.Debug("########## Removing network")
		err := removeNetwork(&ctx, cli, build.Settings.NetworkName)
		if err != nil {
			return Error("Failed to remove network: %s", err)
		}
	}

	if !mustReuseVolume {
		log.Debug("########## Removing volume")
		err := removeVolume(&ctx, cli, build.Settings.VolumeName)
		if err != nil {
			return Error("Failed to remove volume: %s", err)
		}
	}

	return
}
