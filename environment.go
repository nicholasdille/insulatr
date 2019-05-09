package main

import (
	"strings"
)

// ExpandEnvironment expands variable values from the environment
func ExpandEnvironment(variables *[]string, environment []string) (err error) {
	for index, envVarDef := range *variables {
		if !strings.Contains(envVarDef, "=") {
			foundMatch := false
			for _, envVar := range environment {
				pair := strings.Split(envVar, "=")
				if pair[0] == envVarDef {
					(*variables)[index] = envVar
					foundMatch = true
				}
			}
			if !foundMatch {
				err = Error("Unable to find match for environment variable <%s> for global environment", envVarDef)
				return
			}
		}
	}

	return
}

// MergeEnvironment merges two sets of environment variables
func MergeEnvironment(globalEnvironment []string, localEnvironment *[]string) (err error) {
	for index, localEnv := range *localEnvironment {
		localPair := strings.Split(localEnv, "=")

		for _, globalEnv := range globalEnvironment {
			globalPair := strings.Split(globalEnv, "=")

			if len(globalPair) < 2 {
				err = Error("Global environment variable <%s> has not been expanded", globalPair)
			}

			if len(localPair) == 1 && globalPair[0] == localPair[0] {
				(*localEnvironment)[index] = globalEnv
			}
		}
	}

	for _, globalEnv := range globalEnvironment {
		globalPair := strings.Split(globalEnv, "=")

		foundMatch := false
		for _, localEnv := range *localEnvironment {
			localPair := strings.Split(localEnv, "=")

			if globalPair[0] == localPair[0] {
				foundMatch = true
			}
		}

		if !foundMatch {
			*localEnvironment = append(*localEnvironment, globalEnv)
		}
	}
	return
}
