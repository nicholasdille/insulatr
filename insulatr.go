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
var FileFormat = logging.MustStringFormatter(
	`%{time:2006-01-02T15:04:05.999Z-07:00} %{level:.7s} %{message}`,
)

// ConsoleFormat defines the log format for the console backend
var ConsoleFormat = logging.MustStringFormatter(
	`%{color}%{time:15:04:05} %{message}%{color:reset}`,
)

func PrepareLogging(ConsoleLogLevelString string, FileWriter io.Writer) {
	var ConsoleLogLevel logging.Level
	switch ConsoleLogLevelString {
	case "DEBUG":
		ConsoleLogLevel = logging.DEBUG
	case "NOTICE":
		ConsoleLogLevel = logging.NOTICE
	case "INFO":
		ConsoleLogLevel = logging.INFO
	}

	FileBackend := logging.NewLogBackend(FileWriter, "", 0)
	FileBackendFormatter := logging.NewBackendFormatter(FileBackend, FileFormat)
	FileBackendLeveled := logging.AddModuleLevel(FileBackendFormatter)
	FileBackendLeveled.SetLevel(logging.INFO, "")

	ConsoleBackend := logging.NewLogBackend(os.Stdout, "", 0)
	ConsoleBackendFormatter := logging.NewBackendFormatter(ConsoleBackend, ConsoleFormat)
	ConsoleBackendLeveled := logging.AddModuleLevel(ConsoleBackendFormatter)
	ConsoleBackendLeveled.SetLevel(ConsoleLogLevel, "")

	logging.SetBackend(FileBackendLeveled, ConsoleBackendLeveled)
}

// Error logs an error message and returns an error object
func Error(format string, a ...interface{}) (err error) {
	message := fmt.Sprintf(format, a)
	Log.Error(message)
	return errors.New(message)
}

func Run(BuildDefinition *Build) (err error) {
	if _, err := os.Stat(BuildDefinition.Settings.LogDirectory); os.IsNotExist(err) {
		os.Mkdir(BuildDefinition.Settings.LogDirectory, 0755)
	}

	FileWriter, err := os.OpenFile("logs/test.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return Error("Failed to open file: ", err)
	}
	PrepareLogging(BuildDefinition.Settings.ConsoleLogLevel, FileWriter)
	Log.Noticef("Running insulatr version %s built at %s from %s\n", Version, BuildTime, GitCommit)

	err = ExpandEnvironment(&BuildDefinition.Environment, os.Environ())
	if err != nil {
		return Error("Unable to expand global environment: %s", err)
	}
	for Index, Repo := range BuildDefinition.Repositories {
		Log.Debugf("len(BuildDefinition.Repositories)=%d.", len(BuildDefinition.Repositories))
		if len(BuildDefinition.Repositories) > 1 {
			if len(Repo.Directory) == 0 || Repo.Directory == "." {
				return Error("All repositories require the directory node to be set (<.> is not allowed)")
			}
		}

		BuildDefinition.Repositories[Index].WorkingDirectory = BuildDefinition.Settings.WorkingDirectory
		BuildDefinition.Repositories[Index].VolumeName = BuildDefinition.Settings.VolumeName
	}
	for Index, Service := range BuildDefinition.Services {
		if Service.Privileged && !BuildDefinition.Settings.AllowPrivileged {
			return Error("Service <%s> requests privileged container but AllowPrivileged was not specified", Service.Name)
		}

		BuildDefinition.Services[Index].NetworkName = BuildDefinition.Settings.NetworkName

		err = ExpandEnvironment(&Service.Environment, BuildDefinition.Environment)
		if err != nil {
			return Error("Unable to expand environment for service <%s> against global environment: %s", Service.Name, err)
		}
		err = ExpandEnvironment(&Service.Environment, os.Environ())
		if err != nil {
			return Error("Unable to expand environment for service <%s> against process environment: %s", Service.Name, err)
		}
	}
	for Index, Step := range BuildDefinition.Steps {
		if Step.MountDockerSock && !BuildDefinition.Settings.AllowDockerSock {
			return Error("Build step <%s> requests to mount Docker socket but AllowDockerSock was not specified", Step.Name)
		}

		if len(Step.Shell) == 0 {
			BuildDefinition.Steps[Index].Shell = BuildDefinition.Settings.Shell
		}
		if len(Step.WorkingDirectory) == 0 {
			BuildDefinition.Steps[Index].WorkingDirectory = BuildDefinition.Settings.WorkingDirectory
		}
		BuildDefinition.Steps[Index].VolumeName = BuildDefinition.Settings.VolumeName
		BuildDefinition.Steps[Index].NetworkName = BuildDefinition.Settings.NetworkName

		err = MergeEnvironment(BuildDefinition.Environment, &Step.Environment)
		if err != nil {
			return Error("Unable to merge environment for step <%s>: %s", Step.Name, err)
		}
		err = ExpandEnvironment(&Step.Environment, os.Environ())
		if err != nil {
			return Error("Unable to expand environment for step <%s> against process environment: %s", Step.Name, err)
		}
	}

	ctx := context.Background()
	ctxTimeout, cancel := context.WithTimeout(ctx, time.Duration(BuildDefinition.Settings.Timeout)*time.Second)
	defer cancel()

	cli, err := CreateDockerClient(&ctxTimeout)
	if err != nil {
		return Error("Unable to create Docker client: %s", err)
	}

	FailedBuild := false

	if !BuildDefinition.Settings.ReuseVolume {
		Log.Debug("########## Remove volume")
		err = RemoveVolume(&ctxTimeout, cli, BuildDefinition.Settings.VolumeName)
		if err != nil {
			return Error("Failed to remove volume: %s", err)
		}

		Log.Debug("########## Create volume")
		err := CreateVolume(&ctxTimeout, cli, BuildDefinition.Settings.VolumeName, BuildDefinition.Settings.VolumeDriver)
		if err != nil {
			return Error("Failed to create volume: %s", err)
		}
		Log.Debugf("Volume name: %s", BuildDefinition.Settings.VolumeName)
	}

	if !FailedBuild && !BuildDefinition.Settings.ReuseNetwork {
		Log.Debug("########## Remove network")
		err = RemoveNetwork(&ctxTimeout, cli, BuildDefinition.Settings.NetworkName)
		if err != nil {
			return Error("Failed to remove network: %s", err)
		}

		Log.Debug("########## Create network")
		newNetworkID, err := CreateNetwork(&ctxTimeout, cli, BuildDefinition.Settings.NetworkName, BuildDefinition.Settings.NetworkDriver)
		if err != nil {
			err = Error("Failed to create network: %s", err)
			FailedBuild = true
		}
		Log.Debugf("Network ID: %s", newNetworkID)
	}

	if !FailedBuild && len(BuildDefinition.Repositories) > 0 {
		Log.Notice("########## Cloning repositories")
		for Index, Repo := range BuildDefinition.Repositories {
			if Repo.Name == "" {
				err = Error("Repository at index <%d> is missing a name", Index)
				FailedBuild = true
				break
			}

			Log.Noticef("########## Cloning repository <%s>", Repo.Name)

			if Repo.Location == "" {
				err = Error("Repository at index <%d> is missing a location", Repo.Name)
				FailedBuild = true
				break
			}

			err = CloneRepo(&ctxTimeout, cli, Repo)
			if err != nil {
				err = Error("Failed to clone repository <%s>: %s", Repo.Name, err)
				FailedBuild = true
			}
		}
	}

	Services := make(map[string]string)
	if !FailedBuild && len(BuildDefinition.Services) > 0 {
		Log.Notice("########## Starting services")
		for Index, Service := range BuildDefinition.Services {
			if Service.Name == "" {
				err = Error("Service at index <%d> is missing a name", Index)
				FailedBuild = true
				break
			}

			Log.Noticef("########## Starting service <%s>", Service.Name)

			if Service.Image == "" {
				err = Error("Service <%s> is missing an image", Service.Name)
				FailedBuild = true
				break
			}

			var containerID string
			containerID, err = StartService(&ctxTimeout, cli, Service, BuildDefinition)
			if err != nil {
				err = Error("Failed to start service <%s>: %s", Service.Name, err)
				FailedBuild = true
				break
			}

			Services[Service.Name] = containerID
		}
	}

	if !FailedBuild && len(BuildDefinition.Files) > 0 {
		Log.Notice("########## Injecting files")

		if !FailedBuild {
			err = InjectFiles(&ctxTimeout, cli, BuildDefinition.Files, BuildDefinition.Settings.WorkingDirectory, BuildDefinition.Settings.VolumeName)
			if err != nil {
				err = Error("Failed to inject files: %s", err)
				FailedBuild = true
			}
		}
	}

	if !FailedBuild && len(BuildDefinition.Steps) > 0 {
		Log.Notice("########## Running build steps")
		for Index, Step := range BuildDefinition.Steps {
			if Step.Name == "" {
				err = Error("Step at index <%d> is missing a name", Index)
				FailedBuild = true
				break
			}
			if Step.Image == "" {
				err = Error("Step at index <%d> is missing an image", Index)
				FailedBuild = true
				break
			}

			Log.Noticef("########## running step <%s>", Step.Name)

			if len(Step.Commands) == 0 {
				err = Error("Step <%s> is missing commands", Step.Name)
				FailedBuild = true
				break
			}

			err = RunStep(&ctxTimeout, cli, Step, BuildDefinition.Environment)
			if err != nil {
				err = Error("Failed to run build step <%s>: %s", Step.Name, err)
				FailedBuild = true
				break
			}
		}
	}

	if !FailedBuild && len(BuildDefinition.Files) > 0 {
		Log.Notice("########## Extracting files")

		err = ExtractFiles(&ctxTimeout, cli, BuildDefinition.Files, BuildDefinition.Settings.WorkingDirectory, BuildDefinition.Settings.VolumeName)
		if err != nil {
			err = Error("Failed to extract files: %s", err)
			FailedBuild = true
		}
	}

	if len(Services) > 0 {
		for name, containerID := range Services {
			Log.Noticef("########## Stopping service %s", name)

			err = StopService(&ctxTimeout, cli, name, containerID, BuildDefinition.Services)
			if err != nil {
				err = Error("Failed to stop service <%s> with container ID <%s>: %s", name, containerID, err)
				FailedBuild = true
				break
			}

			delete(Services, name)
		}
	}

	if !BuildDefinition.Settings.RetainNetwork {
		Log.Debug("########## Removing network")
		err := RemoveNetwork(&ctx, cli, BuildDefinition.Settings.NetworkName)
		if err != nil {
			return Error("Failed to remove network: %s", err)
		}
	}

	if !BuildDefinition.Settings.RetainVolume {
		Log.Debug("########## Removing volume")
		err := RemoveVolume(&ctx, cli, BuildDefinition.Settings.VolumeName)
		if err != nil {
			return Error("Failed to remove volume: %s", err)
		}
	}

	return
}
