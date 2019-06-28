# insulatr

`insulatr` is a tool for container native builds written in Go.

Based on a YAML file, `insulatr` isolates build steps in individual containers while results are transported using a Docker volume.

## Why `insulatr`

Container native builds facilitate container to execute the individual steps in a build definition. The provides the following advantages:

1. **Runtime environment**: When executing tasks in a container the requirements on the host are reduced to the container runtime. By choosing the container image, the build steps is executed in the appropriate runtime environment.

1. **Isolation**: The tasks executed as part of the build are isolated from each other as well as from the host. It is even possible to use conflicting toolsets for individual steps.

1. **Reproducibility**: Each build step is isolated in a predefined runtime environment and will produce the same behaviour when repeated.

1. **Pipeline as Code**: The build process is specified in a textual form and can be stored in the same repository as the code. Contrary to a build script, it specifies a single execution path.

`insulatr` is deliberately designed as a standalone tool to execute a build definition in containerized steps. Although multiple CI/CD tools and products exist which combine a scheduler with an execution engine, they are tightly coupled. By having a separate tool like `insulatr`, builds can be reproduced in any compatible execution environment - during development as well as in stages of a deployment.

## Table of contents

1. [Usage](#usage)
    1. [Local](#local)
    1. [Docker image](#docker-image)
    1. [Alias](#alias)
1. [Build definitions](docs/build-definitions.md)
1. [Building](#building)
1. [Design](docs/design.md)
1. [Useful links](#useful-links)

## Usage

`insulatr` supports different ways to launch.

### Local

When calling `insulatr` without any parameters, it will look for a file called `insulatr.yaml` in the current directory.

The following parameters are supported:

```
Options:

  -h, --help                        display help information
  -f, --file[=./insulatr.yaml]      Build definition file
      --reuse-volume[=false]        Use existing volume
      --retain-volume[=false]       Retain existing volume
      --reuse-network[=false]       Use existing network
      --retain-network[=false]      Retain existing network
      --reuse[=false]               Same as --reuse-volume and --reuse-network
      --remove[=false]              Same as --retain-volume and --retain-network
      --allow-docker-sock[=false]   Allow docker socket in build steps
      --allow-privileged[=false]    Allow privileged container for services
```

### Docker image

The Docker image [`nicholasdille/insulatr`](https://cloud.docker.com/repository/docker/nicholasdille/insulatr) is [automatically built by Docker Hub](https://cloud.docker.com/repository/docker/nicholasdille/insulatr/builds). `insulatr` ships as a scratch image with only the statically linked binary.

The following tags are currently supported:

- [`master` (Dockerfile#master)](https://github.com/nicholasdille/insulatr/blob/master/Dockerfile)
- [`1.0.2`, `1.0`, `1`, `latest` (Dockerfile#1.0.2)](https://github.com/nicholasdille/insulatr/blob/1.0.2/Dockerfile)

New releases receive a git tag which triggers a separate build which produces a new image tagged with the versions.

The Docker image is used in the following way:

```bash
docker run -it --rm --volume $(pwd)/insulatr.yaml:/insulatr.yaml --volume /var/run/docker.sock:/var/run/docker.sock nicholasdille/insulatr [<parameters>]
```

### Alias

If the Docker daemon is running on a remote host, the following alias will add the local `insulatr.yaml` to a new image and run it:

```bash
alias insulatr="echo -e 'FROM nicholasdille/insulatr\nADD insulatr.yaml /' | docker image build --file - --tag insulatr:test --quiet . | xargs -r docker run -t -v /var/run/docker.sock:/var/run/docker.sock"
```

## Building

The following commands build `insulatr` from source.

1. Clone repository: `git clone https://github.com/nicholasdille/insulatr`
1. Download dependencies: `make deps`
1. Build static binary: `make static`

The resulting binary is located in `bin/insulatr-x86_64`.

## Useful links

[Docker Go SDK](https://godoc.org/github.com/docker/docker/client)

[Docker Go Examples](https://docs.docker.com/develop/sdk/examples/)

[GitLab Runner Docker Executor](https://gitlab.com/gitlab-org/gitlab-runner/blob/master/executors/docker/executor_docker.go#L1038)

[Docker CLI](https://github.com/docker/cli/blob/master/cli/command/container/run.go#L268)
