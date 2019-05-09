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
	VolumeName       string   `yaml:"volume_name"`
	VolumeDriver     string   `yaml:"volume_driver"`
	WorkingDirectory string   `yaml:"working_directory"`
	Shell            []string `yaml:"shell"`
	NetworkName      string   `yaml:"network_name"`
	NetworkDriver    string   `yaml:"network_driver"`
	Timeout          int      `yaml:"timeout"`
	LogDirectory     string   `yaml:"log_directory"`
	ConsoleLogLevel  string   `yaml:"console_log_level"`
	ReuseVolume      bool     `yaml:"reuse_volume"`
	RetainVolume     bool     `yaml:"retain_volume"`
	ReuseNetwork     bool     `yaml:"reuse_network"`
	RetainNetwork    bool     `yaml:"retain_network"`
	AllowPrivileged  bool
	AllowDockerSock  bool
}

// Repository is used to import from YaML
type Repository struct {
	Name             string `yaml:"name"`
	Location         string `yaml:"location"`
	Directory        string `yaml:"directory"`
	Shallow          bool   `yaml:"shallow"`
	Branch           string `yaml:"branch"`
	Tag              string `yaml:"tag"`
	Commit           string `yaml:"commit"`
	WorkingDirectory string
	VolumeName       string
}

// Service is used to import from YaML
type Service struct {
	Name        string   `yaml:"name"`
	Image       string   `yaml:"image"`
	Environment []string `yaml:"environment"`
	SuppressLog bool     `yaml:"suppress_log"`
	Privileged  bool     `yaml:"privileged"`
	NetworkName string
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
	WorkingDirectory   string   `yaml:"working_directory"`
	VolumeName         string
	NetworkName        string
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

// GetBuildDefinitionDefaults presets defaults values for a build definition
func GetBuildDefinitionDefaults() *Build {
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
			ConsoleLogLevel:  "NOTICE",
		},
	}
}

// Log contains the global logger
var Log = logging.MustGetLogger("insulatr")

// FileFormat defines the log format for the file backend
var fileFormat = logging.MustStringFormatter(
	`%{time:2006-01-02T15:04:05.999Z-07:00} %{level:.7s} %{message}`,
)

// ConsoleFormat defines the log format for the console backend
var consoleFormat = logging.MustStringFormatter(
	`%{color}%{time:15:04:05} %{message}%{color:reset}`,
)

// PrepareLogging create the logging system with file and console backends
func PrepareLogging(consoleLogLevelString string, fileWriter io.Writer) {
	var consoleLogLevel logging.Level
	switch consoleLogLevelString {
	case "DEBUG":
		consoleLogLevel = logging.DEBUG
	case "NOTICE":
		consoleLogLevel = logging.NOTICE
	case "INFO":
		consoleLogLevel = logging.INFO
	}

	fileBackend := logging.NewLogBackend(fileWriter, "", 0)
	fileBackendFormatter := logging.NewBackendFormatter(fileBackend, fileFormat)
	fileBackendLeveled := logging.AddModuleLevel(fileBackendFormatter)
	fileBackendLeveled.SetLevel(logging.INFO, "")

	consoleBackend := logging.NewLogBackend(os.Stdout, "", 0)
	consoleBackendFormatter := logging.NewBackendFormatter(consoleBackend, consoleFormat)
	consoleBackendLeveled := logging.AddModuleLevel(consoleBackendFormatter)
	consoleBackendLeveled.SetLevel(consoleLogLevel, "")

	logging.SetBackend(fileBackendLeveled, consoleBackendLeveled)
}

// Error logs an error message and returns an error object
func Error(format string, a ...interface{}) (err error) {
	message := fmt.Sprintf(format, a)
	Log.Error(message)
	return errors.New(message)
}

// Run executes the build definition
func Run(buildDefinition *Build) (err error) {
	if _, err := os.Stat(buildDefinition.Settings.LogDirectory); os.IsNotExist(err) {
		os.Mkdir(buildDefinition.Settings.LogDirectory, 0755)
	}

	fileWriter, err := os.OpenFile("logs/test.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return Error("Failed to open file: ", err)
	}
	PrepareLogging(buildDefinition.Settings.ConsoleLogLevel, fileWriter)
	Log.Noticef("Running insulatr version %s built at %s from %s\n", version, buildTime, gitCommit)

	err = ExpandEnvironment(&buildDefinition.Environment, os.Environ())
	if err != nil {
		return Error("Unable to expand global environment: %s", err)
	}
	for index, repo := range buildDefinition.Repositories {
		Log.Debugf("len(buildDefinition.Repositories)=%d.", len(buildDefinition.Repositories))
		if len(buildDefinition.Repositories) > 1 {
			if len(repo.Directory) == 0 || repo.Directory == "." {
				return Error("All repositories require the directory node to be set (<.> is not allowed)")
			}
		}

		buildDefinition.Repositories[index].WorkingDirectory = buildDefinition.Settings.WorkingDirectory
		buildDefinition.Repositories[index].VolumeName = buildDefinition.Settings.VolumeName
	}
	for index, service := range buildDefinition.Services {
		if service.Privileged && !buildDefinition.Settings.AllowPrivileged {
			return Error("Service <%s> requests privileged container but AllowPrivileged was not specified", service.Name)
		}

		buildDefinition.Services[index].NetworkName = buildDefinition.Settings.NetworkName

		err = ExpandEnvironment(&service.Environment, buildDefinition.Environment)
		if err != nil {
			return Error("Unable to expand environment for service <%s> against global environment: %s", service.Name, err)
		}
		err = ExpandEnvironment(&service.Environment, os.Environ())
		if err != nil {
			return Error("Unable to expand environment for service <%s> against process environment: %s", service.Name, err)
		}
	}
	for index, step := range buildDefinition.Steps {
		if step.MountDockerSock && !buildDefinition.Settings.AllowDockerSock {
			return Error("Build step <%s> requests to mount Docker socket but AllowDockerSock was not specified", step.Name)
		}

		if len(step.Shell) == 0 {
			log.Warningf("Parameter <shell> of step <%s> is overwritten by global setting", step.Name)
			buildDefinition.Steps[index].Shell = buildDefinition.Settings.Shell
		}
		if len(step.WorkingDirectory) == 0 {
			log.Warningf("Parameter <working_directory> of step <%s> is overwritten by global setting", step.Name)
			buildDefinition.Steps[index].WorkingDirectory = buildDefinition.Settings.WorkingDirectory
		}
		buildDefinition.Steps[index].VolumeName = buildDefinition.Settings.VolumeName
		buildDefinition.Steps[index].NetworkName = buildDefinition.Settings.NetworkName

		err = MergeEnvironment(buildDefinition.Environment, &step.Environment)
		if err != nil {
			return Error("Unable to merge environment for step <%s>: %s", step.Name, err)
		}
		err = ExpandEnvironment(&step.Environment, os.Environ())
		if err != nil {
			return Error("Unable to expand environment for step <%s> against process environment: %s", step.Name, err)
		}
	}

	ctx := context.Background()
	ctxTimeout, cancel := context.WithTimeout(ctx, time.Duration(buildDefinition.Settings.Timeout)*time.Second)
	defer cancel()

	cli, err := CreateDockerClient(&ctxTimeout)
	if err != nil {
		return Error("Unable to create Docker client: %s", err)
	}

	failedBuild := false

	if !buildDefinition.Settings.ReuseVolume {
		Log.Debug("########## Remove volume")
		err = RemoveVolume(&ctxTimeout, cli, buildDefinition.Settings.VolumeName)
		if err != nil {
			return Error("Failed to remove volume: %s", err)
		}

		Log.Debug("########## Create volume")
		err := CreateVolume(&ctxTimeout, cli, buildDefinition.Settings.VolumeName, buildDefinition.Settings.VolumeDriver)
		if err != nil {
			return Error("Failed to create volume: %s", err)
		}
		Log.Debugf("Volume name: %s", buildDefinition.Settings.VolumeName)
	}

	if !failedBuild && !buildDefinition.Settings.ReuseNetwork {
		Log.Debug("########## Remove network")
		err = RemoveNetwork(&ctxTimeout, cli, buildDefinition.Settings.NetworkName)
		if err != nil {
			return Error("Failed to remove network: %s", err)
		}

		Log.Debug("########## Create network")
		newNetworkID, err := CreateNetwork(&ctxTimeout, cli, buildDefinition.Settings.NetworkName, buildDefinition.Settings.NetworkDriver)
		if err != nil {
			err = Error("Failed to create network: %s", err)
			failedBuild = true
		}
		Log.Debugf("Network ID: %s", newNetworkID)
	}

	if !failedBuild && len(buildDefinition.Repositories) > 0 {
		Log.Notice("########## Cloning repositories")
		for index, repo := range buildDefinition.Repositories {
			if repo.Name == "" {
				err = Error("Repository at index <%d> is missing a name", index)
				failedBuild = true
				break
			}

			Log.Noticef("########## Cloning repository <%s>", repo.Name)

			if repo.Location == "" {
				err = Error("Repository at index <%d> is missing a location", repo.Name)
				failedBuild = true
				break
			}

			err = CloneRepo(&ctxTimeout, cli, repo)
			if err != nil {
				err = Error("Failed to clone repository <%s>: %s", repo.Name, err)
				failedBuild = true
			}
		}
	}

	services := make(map[string]string)
	if !failedBuild && len(buildDefinition.Services) > 0 {
		Log.Notice("########## Starting services")
		for index, service := range buildDefinition.Services {
			if service.Name == "" {
				err = Error("Service at index <%d> is missing a name", index)
				failedBuild = true
				break
			}

			Log.Noticef("########## Starting service <%s>", service.Name)

			if service.Image == "" {
				err = Error("Service <%s> is missing an image", service.Name)
				failedBuild = true
				break
			}

			var containerID string
			containerID, err = StartService(&ctxTimeout, cli, service, buildDefinition)
			if err != nil {
				err = Error("Failed to start service <%s>: %s", service.Name, err)
				failedBuild = true
				break
			}

			services[service.Name] = containerID
		}
	}

	if !failedBuild && len(buildDefinition.Files) > 0 {
		Log.Notice("########## Injecting files")

		if !failedBuild {
			err = InjectFiles(&ctxTimeout, cli, buildDefinition.Files, buildDefinition.Settings.WorkingDirectory, buildDefinition.Settings.VolumeName)
			if err != nil {
				err = Error("Failed to inject files: %s", err)
				failedBuild = true
			}
		}
	}

	if !failedBuild && len(buildDefinition.Steps) > 0 {
		Log.Notice("########## Running build steps")
		for index, step := range buildDefinition.Steps {
			if step.Name == "" {
				err = Error("Step at index <%d> is missing a name", index)
				failedBuild = true
				break
			}
			if step.Image == "" {
				err = Error("Step at index <%d> is missing an image", index)
				failedBuild = true
				break
			}

			Log.Noticef("########## running step <%s>", step.Name)

			if len(step.Commands) == 0 {
				err = Error("Step <%s> is missing commands", step.Name)
				failedBuild = true
				break
			}

			err = RunStep(&ctxTimeout, cli, step, buildDefinition.Environment)
			if err != nil {
				err = Error("Failed to run build step <%s>: %s", step.Name, err)
				failedBuild = true
				break
			}
		}
	}

	if !failedBuild && len(buildDefinition.Files) > 0 {
		Log.Notice("########## Extracting files")

		err = ExtractFiles(&ctxTimeout, cli, buildDefinition.Files, buildDefinition.Settings.WorkingDirectory, buildDefinition.Settings.VolumeName)
		if err != nil {
			err = Error("Failed to extract files: %s", err)
			failedBuild = true
		}
	}

	if len(services) > 0 {
		for name, id := range services {
			Log.Noticef("########## Stopping service %s", name)

			err = StopService(&ctxTimeout, cli, name, id, buildDefinition.Services)
			if err != nil {
				err = Error("Failed to stop service <%s> with container ID <%s>: %s", name, id, err)
				failedBuild = true
				break
			}

			delete(services, name)
		}
	}

	if !buildDefinition.Settings.RetainNetwork {
		Log.Debug("########## Removing network")
		err := RemoveNetwork(&ctx, cli, buildDefinition.Settings.NetworkName)
		if err != nil {
			return Error("Failed to remove network: %s", err)
		}
	}

	if !buildDefinition.Settings.RetainVolume {
		Log.Debug("########## Removing volume")
		err := RemoveVolume(&ctx, cli, buildDefinition.Settings.VolumeName)
		if err != nil {
			return Error("Failed to remove volume: %s", err)
		}
	}

	return
}
