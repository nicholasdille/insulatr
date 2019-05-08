package main

import (
	"os"
	"strings"
)

func ExpandEnvironment(environment *[]string) (err error) {
	for index, envVarDef := range *environment {
		if !strings.Contains(envVarDef, "=") {
			FoundMatch := false
			for _, envVar := range os.Environ() {
				pair := strings.Split(envVar, "=")
				if pair[0] == envVarDef {
					(*environment)[index] = envVar
					FoundMatch = true
				}
			}
			if !FoundMatch {
				err = Error("Unable to find match for environment variable <%s> for global environment", envVarDef)
				return
			}
		}
	}

	return
}

func MergeEnvironment(GlobalEnvironment []string, LocalEnvironment *[]string) (err error) {
	for index, LocalEnv := range *LocalEnvironment {
		LocalPair := strings.Split(LocalEnv, "=")

		for _, GlobalEnv := range GlobalEnvironment {
			GlobalPair := strings.Split(GlobalEnv, "=")

			if len(GlobalPair) < 2 {
				err = Error("Global environment variable <%s> has not been expanded", GlobalPair)
			}

			if len(LocalPair) == 1 && GlobalPair[0] == LocalPair[0] {
				(*LocalEnvironment)[index] = GlobalEnv
			}
		}
	}

	for _, GlobalEnv := range GlobalEnvironment {
		GlobalPair := strings.Split(GlobalEnv, "=")

		FoundMatch := false
		for _, LocalEnv := range *LocalEnvironment {
			LocalPair := strings.Split(LocalEnv, "=")

			if GlobalPair[0] == LocalPair[0] {
				FoundMatch = true
			}
		}

		if !FoundMatch {
			*LocalEnvironment = append(*LocalEnvironment, GlobalEnv)
		}
	}
	return
}

func expandGlobalEnvironment(build *Build) (err error) {
	for index, envVarDef := range build.Environment {
		if !strings.Contains(envVarDef, "=") {
			FoundMatch := false
			for _, envVar := range os.Environ() {
				pair := strings.Split(envVar, "=")
				if pair[0] == envVarDef {
					build.Environment[index] = envVar
					FoundMatch = true
				}
			}
			if !FoundMatch {
				return Error("Unable to find match for environment variable <%s> for global environment", envVarDef)
			}
		}
	}

	return
}
