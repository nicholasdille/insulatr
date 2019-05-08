package main

import (
	"os"
	"strings"
)

func ExpandEnvironment(environment []string) (ExpandedEnvironment []string, err error) {
	for _, envVarDef := range environment {
		if !strings.Contains(envVarDef, "=") {
			FoundMatch := false
			for _, envVar := range os.Environ() {
				pair := strings.Split(envVar, "=")
				if pair[0] == envVarDef {
					ExpandedEnvironment = append(ExpandedEnvironment, envVar)
					FoundMatch = true
				}
			}
			if !FoundMatch {
				err = Error("Unable to find match for environment variable <%s> for global environment", envVarDef)
				return
			}

		} else {
			ExpandedEnvironment = append(ExpandedEnvironment, envVarDef)
		}
	}

	return
}

func MergeEnvironment(GlobalEnvironment []string, LocalEnvironment []string) (MergedEnvironment []string, err error) {
	for _, GlobalEnv := range GlobalEnvironment {
		GlobalPair := strings.Split(GlobalEnv, "=")

		FoundMatch := false
		for _, LocalEnv := range LocalEnvironment {
			LocalPair := strings.Split(LocalEnv, "=")

			if GlobalPair[0] == LocalPair[0] {
				MergedEnvironment = append(MergedEnvironment, LocalEnv)
				FoundMatch = true
			}
		}

		if !FoundMatch {
			MergedEnvironment = append(MergedEnvironment, GlobalEnv)
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
