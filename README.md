# insulatr

`insulatr` is a tool for container native builds written in Go.

Based on a YAML file, `insulatr` isolates build steps in individual containers while results are transported using a Docker volume.

## Table of contents

1. [Why `insulatr`](#why-insulatr)
1. [Usage](#usage)
1. [Build definitions](#build-definitions)
1. [Building](#building)
1. [Design](#design)
1. [Useful links](#useful-links)

## Why `insulatr`

`insulatr` enables container native builds without the requirement of a CI/CD tool. Although the tight integration of scheduling and pipeline-as-code is beneficial, being able to choose separate tools for the job is a nice thing.

## Usage

When calling `insulatr` without any parameters, it will look for a file called `insulatr.yaml` in the current directory.

The following parameters are supported:

```
Options:

  -h, --help                        display help information
  -f, --file[=./insulatr.yaml]      Build definition file
      --reuse-volume[=false]        Use existing volume
      --remove-volume[=false]       Remove existing volume
      --reuse-network[=false]       Use existing network
      --remove-network[=false]      Remove existing network
      --reuse[=false]               Same as --reuse-volume and --reuse-network
      --remove[=false]              Same as --remove-volume and --remove-network
      --allow-docker-sock[=false]   Allow docker socket in build steps
      --allow-privileged[=false]    Allow privileged container for services
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

The `settings` node defines global configuration options. It supports the following (optional) fields:

- `volume_name` contains the name of the volume transporting repository checkouts as well as builds results across the build steps. It defaults to `myvolume`.
- `volume_driver` specifies the volume driver to use. It defaults to `local`.
- `working_directory` contains the path under which to mount the volume. It defaults to `/src`.
- `shell` is an array specifying the shell to run commands under. It defaults to `[ "sh" ]` to support minimized distribution images.
- `network_name` contains the name of the network to connect services as well as build steps with. It defaults to `mynetwork`.
- `network_driver` specifies the network driver to use. It defaults to `bridge`.
- `timeout` defines how long to wait (in seconds) for the whole build before failing. It defaults to `3600`.

To summarize, the default settings are:

```yaml
settings:
  volume_name: myvolume
  volume_driver: local
  working_directory: /src
  shell: [ "sh" ]
  network_name: mynetwork
  network_driver: bridge
  timeout: 60
```

### Repositories

The `repos` node defines a list of Git repositories to checkout before executing build steps. Currently, only unauthorized repositories are supported. The following fields are supported per repository:

- `name` (mandatory) contains the given name for a repository.
- `location` (mandatory) contains the URL to the repository.
- `directory` (optional) contains the directory to checkout into. If omitted, the checkout behaves as `git clone <url>` and creates a new directory with a name based on the repository name.
- `shallow` (optional) specifies whether to create a shallow clone. It defaults to `false`.
- `branch` (optional) specifies a branch to checkout.
- `tag` (optional) specifies a tag to checkout.
- `commit` (optional) specifies a commit to checkout.

A typical repository definition looks like this:

```yaml
repos:
  - name: main
    location: https://github.com/nicholasdille/insulatr
```

### Services

The `services` node defines a list of services required by the build steps. The are started in order before build steps are executed. The following fields are supported per service:

- `name` (mandatory) contains the given name for a repository.
- `image` (mandatory) specifies the image to run the services with.
- `environment` (optional) defines the environment variables required to configure the service.
- `privileged` (optional) specifies whether the container will be privileged. It defaults to `false`.

A typical service definition looks like this:

```yaml
services:
  - name: web
    image: nginx
```

### Build steps

The `steps` node defines a list of build steps to execute. XXX.

- `name` (mandatory) contains the given name of a build step.
- `image` (mandatory) specifies the image to run the step with.
- `commands` (mandatory) is a list of commands to execute in the build step.
- `environment` (optional) defines the environment variables passed to the build step.
- `user` (optional) is a user to execute the commands under.
- `shell` (optional) overrides the [global `shell` setting](#settings).
- `override_entrypoint` (optional) executes the shell as the entrypoint. It defaults to `false`.
- `mount_docker_sock` (optional) mounts `/var/run/docker.sock` into the container. It defaults to `false`.

Typical build steps look like this:

```yaml
steps:
  - name: build
    image: alpine
    environment:
      - FOO=bar
    commands:
      - printenv
```

### Example

```yaml
settings:
  volume_name: myvolume
  working_directory: /src
  shell: [ "sh", "-x", "-e" ]
  network_name: mynetwork

repos:
  - name: main
    location: https://github.com/docker/app
    shallow: true
    directory: app
  - name: main
    location: https://github.com/docker/distribution
    shallow: true
    directory: distribution

services:
  - name: dind
    image: docker:dind
    privileged: true

steps:

  - name: user
    image: alpine
    user: 1000
    commands:
      - id -u

  - name: build
    image: docker:stable
    environment:
      - DOCKER_HOST=tcp://dind:2375
    commands:
      - printenv
      - docker version

  - name: dood
    image: docker:stable
    override_entrypoint: true
    mount_docker_sock: true
    commands:
      - docker version
```

## Building

The following commands build `insulatr` from source.

1. Clone repository: `git clone https://github.com/nicholasdille/insulatr`
1. Download dependencies: `make deps`
1. Build static binary: `make static`

The resulting binary is located in `bin/insulatr-x86_64`.

## Design

This sections lists internals about `insulatr`.

### Execution order

The order of the sections is:

1. `services`
1. `repos`
1. `steps`

## Useful links

[Docker Go SDK](https://godoc.org/github.com/docker/docker/client)

[Docker Go Examples](https://docs.docker.com/develop/sdk/examples/)

[GitLab Runner Docker Executor](https://gitlab.com/gitlab-org/gitlab-runner/blob/master/executors/docker/executor_docker.go#L1038)

[Docker CLI](https://github.com/docker/cli/blob/master/cli/command/container/run.go#L268)
