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
	File          string `cli:"f,file"         usage:"Build definition file"                        dft:"./insulatr.yaml"`
	ReuseVolume   bool   `cli:"reuse-volume"   usage:"Use existing volume"                          dft:"false"`
	RemoveVolume  bool   `cli:"remove-volume"  usage:"Remove existing volume"                       dft:"false"`
	ReuseNetwork  bool   `cli:"reuse-network"  usage:"Use existing network"                         dft:"false"`
	RemoveNetwork bool   `cli:"remove-network" usage:"Remove existing network"                      dft:"false"`
	Reuse         bool   `cli:"reuse"          usage:"Same as --reuse-volume and --reuse-network"   dft:"false"`
	Remove        bool   `cli:"remove"         usage:"Same as --remove-volume and --remove-network" dft:"false"`
}

var GitCommit string
var BuildTime string
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
        fmt.Printf("Running insulatr version %s built at %s from %s\n", Version, BuildTime, GitCommit)

	os.Exit(cli.Run(new(argT), func(ctx *cli.Context) error {
		argv := ctx.Argv().(*argT)

		_, err := os.Stat(argv.File)
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: File %s does not exist.\n", argv.File)
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

		err = run(build, argv.ReuseVolume, argv.RemoveVolume, argv.ReuseNetwork, argv.RemoveNetwork)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error building %s: %s\n", argv.File, err)
			os.Exit(1)
		}

		return nil
	}))
}
