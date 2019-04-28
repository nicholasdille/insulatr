# insulatr

`insulatr` is a tool for container native builds written in Go.

Based on a YAML file, `insulatr` isolates build steps in individual containers while results are transported using a Docker volume.

## Table of contents

1. [Usage](#usage)
1. [Build definitions](#build-definitions)
1. [Building](#building)
1. [Design](#design)
1. [Useful links](#useful-links)

## Usage

When calling `insulatr` without any parameters, it will look for a file called `insulatr.yaml` in the current directory.

The following parameters are supported:

```
Options:

  -h, --help                     display help information
  -f, --file[=./insulatr.yaml]   Build definition file
      --reuse-volume[=false]     Use existing volume
      --remove-volume[=false]    Remove existing volume
      --reuse-network[=false]    Use existing network
      --remove-network[=false]   Remove existing network
      --reuse[=false]            Same as --reuse-volume and --reuse-network
      --remove[=false]           Same as --remove-volume and --remove-network
```

### Docker image

The Docker image [`nicholasdille/insulatr`](https://cloud.docker.com/repository/docker/nicholasdille/insulatr) is [automatically built by Docker Hub](https://cloud.docker.com/repository/docker/nicholasdille/insulatr/builds). `insulatr` ships as a scratch image with only the statically linked binary.

The following tags are currently supported:

- [`latest` (Dockerfile#master)](https://github.com/nicholasdille/insulatr/blob/master/Dockerfile)

New releases receive a git tag which triggers a separate build which produces a new image tagged with the versions.

The Docker image is used in the following way:

```bash
docker run -it --volume $PWD:/insulatr --workdir /insulatr nicholasdille/insulatr [<parameters>]
```

### Alias

If the Docker daemon is running on a remote host, the following alias will add the local `insulatr.yaml` to a new image and run it:

```bash
alias insulatr="echo -e 'FROM nicholasdille/insulatr\nADD insulatr.yaml /' | docker image build --file - --tag insulatr:test --quiet . | xargs -r docker run -t -v /var/run/docker.sock:/var/run/docker.sock"
```

## Build definitions

`insulatr` requires a build definition written in YAML.

### Settings

XXX

### Repositories

XXX

### Services

XXX

### Build steps

XXX

### Example

```
settings:
  volume_name: myvolume
  working_directory: /src
  shell: [ "sh", "-x", "-e" ]
  network_name: mynetwork

repos:
  - name: main
    location: https://github.com/docker/app
    shallow: true
    directory: .

services:
  - name: web
    image: nginx

steps:

  - name: test
    image: alpine
    environment:
      TEST: foobar
    commands:
      - printenv
      - pwd
      - ls -l
      - ip a
      - df
      - test -f .git/shallow

  - name: web
    image: alpine
    commands:
      - apk update
      - apk add curl
      - curl -s web

  - name: user
    image: alpine
    user: 1000
    commands:
      - id -u

  - name: entrypoint
    image: alpine/git
    override_entrypoint: true
    commands:
      - pwd
```

## Building

XXX

## Design

XXX

### Design decisions

XXX

## Useful links

[Docker Go SDK](https://godoc.org/github.com/docker/docker/client)

[Docker Go Examples](https://docs.docker.com/develop/sdk/examples/)

[GitLab Runner Docker Executor](https://gitlab.com/gitlab-org/gitlab-runner/blob/master/executors/docker/executor_docker.go#L1038)

[Docker CLI](https://github.com/docker/cli/blob/master/cli/command/container/run.go#L268)
