package main

import (
	"strings"
)

// ExpandEnvironment expands variable values from the environment
func ExpandEnvironment(variables *[]string, environment []string) (err error) {
	for index, envVarDef := range *variables {
		if !strings.Contains(envVarDef, "=") {
			FoundMatch := false
			for _, envVar := range environment {
				pair := strings.Split(envVar, "=")
				if pair[0] == envVarDef {
					(*variables)[index] = envVar
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

// MergeEnvironment merges two sets of environment variables
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
