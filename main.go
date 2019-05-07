package main

import (
	"fmt"
	"github.com/mkideal/cli"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
)

type argT struct {
	cli.Helper
	File            string `cli:"f,file"              usage:"Build definition file"                        dft:"./insulatr.yaml"`
	ReuseVolume     bool   `cli:"reuse-volume"        usage:"Use existing volume"                          dft:"false"`
	RemoveVolume    bool   `cli:"remove-volume"       usage:"Remove existing volume"                       dft:"false"`
	ReuseNetwork    bool   `cli:"reuse-network"       usage:"Use existing network"                         dft:"false"`
	RemoveNetwork   bool   `cli:"remove-network"      usage:"Remove existing network"                      dft:"false"`
	Reuse           bool   `cli:"reuse"               usage:"Same as --reuse-volume and --reuse-network"   dft:"false"`
	Remove          bool   `cli:"remove"              usage:"Same as --remove-volume and --remove-network" dft:"false"`
	AllowDockerSock bool   `cli:"allow-docker-sock"   usage:"Allow docker socket in build steps"           dft:"false"`
	AllowPrivileged bool   `cli:"allow-privileged"    usage:"Allow privileged container for services"      dft:"false"`
	ConsoleLogLevel string `cli:"l,console-log-level" usage:"Controls the log level on the console"`
}

// GitCommit will be filled from build flags
var GitCommit string

// BuildTime will be filled from build flags
var BuildTime string

// Version will be filled from build flags
var Version string

func main() {
	if len(GitCommit) == 0 {
		GitCommit = "UNKNOWN"
	}
	if len(BuildTime) == 0 {
		BuildTime = "UNKNOWN"
	}
	if len(Version) == 0 {
		Version = "UNKNOWN"
	}

	os.Exit(cli.Run(new(argT), func(ctx *cli.Context) error {
		argv := ctx.Argv().(*argT)

		_, err := os.Stat(argv.File)
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: File <%s> does not exist.\n", argv.File)
			os.Exit(1)
		}
		source, err := ioutil.ReadFile(argv.File)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file %s: %s\n", argv.File, err)
			os.Exit(1)
		}

		build := defaults()
		err = yaml.Unmarshal(source, &build)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing YAML: %s\n", err)
			os.Exit(1)
		}

		if argv.Reuse {
			argv.ReuseVolume = true
			argv.ReuseNetwork = true
		}

		if (argv.Reuse && argv.Remove) ||
			(argv.ReuseVolume && argv.RemoveVolume) ||
			(argv.ReuseNetwork && argv.RemoveNetwork) {
			fmt.Fprintf(os.Stderr, "Error: Cannot reuse volume/network if instructed to remove them.\n")
			os.Exit(1)
		}

		switch argv.ConsoleLogLevel {
		case "DEBUG", "NOTICE", "INFO":
			build.Settings.ConsoleLogLevel = argv.ConsoleLogLevel
		case "":
		default:
			fmt.Fprintf(os.Stderr, "Console log level must be DEBUG, NOTICE or INFO (got: %s)\n", argv.ConsoleLogLevel)
			os.Exit(1)
		}

		run(build, argv.ReuseVolume, argv.RemoveVolume, argv.ReuseNetwork, argv.RemoveNetwork, argv.AllowDockerSock, argv.AllowPrivileged)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error building %s: %s\n", argv.File, err)
			os.Exit(1)
		}

		return nil
	}))
}
