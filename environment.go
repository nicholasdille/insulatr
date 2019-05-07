package main

import (
	"fmt"
	"os"
	"strings"
)

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
				message := fmt.Sprintf("Unable to find match for environment variable <%s> for global environment", envVarDef)
				log.Error(message)
				return errors.New(message)
			}
		}
	}

	return
}