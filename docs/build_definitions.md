# Build definitions

`insulatr` requires a build definition written in YAML with the following sections:

1. [Settings](#settings)
1. [Environment](#environment)
1. [Repositories](#repositories)
1. [Files](#files)
1. [Services](#services)
1. [Build Steps](#build-steps)

## Settings

The `settings` node defines global configuration options. It supports the following (optional) fields:

- `volume_name` contains the name of the volume transporting repository checkouts as well as builds results across the build steps. It defaults to `myvolume`.
- `volume_driver` specifies the volume driver to use. It defaults to `local`.
- `working_directory` contains the path under which to mount the volume. It defaults to `/src`.
- `shell` is an array specifying the shell to run commands under. It defaults to `[ "sh" ]` to support minimized distribution images.
- `network_name` contains the name of the network to connect services as well as build steps with. It defaults to `mynetwork`.
- `network_driver` specifies the network driver to use. It defaults to `bridge`.
- `timeout` defines how long to wait (in seconds) for the whole build before failing. It defaults to `3600`.
- `log_directory` specifies the directory to store logs in. It defaults to `logs`.
- `console_log_level` controls what level of messages are displayed. Valid values are `NOTICE`, `INFO`, `DEBUG`. IT defaults to `NOTICE`.
- `reuse_volume` defines whether the volume may be reused if it already exists. It defaults to `false`.
- `retain_volume` defines whether the volume may not be deleted. It defaults to `false`.
- `reuse_network` defines whether the network may be reused if it already exists. It defaults to `false`.
- `retain_network` defines whether the network may not be deleted. It defaults to `false`.

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
  log_directory: logs
  console_log_level: NOTICE
  reuse_volume: false
  retain_volume: false
  reuse_network: false
  retain_network: false
```

## Environment

The `environment` node defines a list of global environment variables. They are automatically added to every build step and can be added to services:

```yaml
environment:
  - FOO=bar

services:
  - name: backend
    image: myimage
    environment:
      - FOO

steps:
  - name: build
    image: myotherimage
    commands:
      - printenv
```

Variables names without values will be resolved against the environment of `insulatr`.

## Repositories

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

Git repositories can be accessed using HTTPS or SSH. Currently, credentials for HTTPS are not supported and you are strongly discouraged from hardcoding the credentials in plaintext in the build definition. For SSH, the agent socket is mapped into the container so that public key authentication will work.

Note that you can use the following URL to clone from GitHub using SSH without authenticating: `git://github.com/<username>/<repo>.git`

## Files

The `files` node defines a list of files to be injected into the volume before running the build steps as well as extracted after the build steps completed successfully. A typical definitions looks like this:

```yaml
files:
  - inject: "Makefile"
  - inject: "*.jar"
  - inject: foo.txt
    content: foobar
  - inject: bar.txt
    content: |-
      foo
      bar
  - extract: bar.txt
```

The `inject` type can be used in two ways:

1. If `content` is set, a new file is created in the volume
1. If `content` is omited, locally existing files and directory are injected. The only supported wildcard is `*`.

When adding a whole directory, the following works...

```yaml
files:
  - inject: go
```

... but the following if `go/` does not exist in the volume...

```yaml
files:
  - inject: go/*
```

## Services

The `services` node defines a list of services required by the build steps. The are started in order before build steps are executed. The following fields are supported per service:

- `name` (mandatory) contains the given name for a repository.
- `image` (mandatory) specifies the image to run the services with.
- `environment` (optional) defines the environment variables required to configure the service.
- `suppress_log` (optional) specifies whether the logs will be displayed when the service is stopped.
- `privileged` (optional) specifies whether the container will be privileged. It defaults to `false`.

A typical service definition looks like this:

```yaml
services:
  - name: web
    image: nginx
```

## Build steps

The `steps` node defines a list of build steps to execute. XXX.

- `name` (mandatory) contains the given name of a build step.
- `image` (mandatory) specifies the image to run the step with.
- `commands` (mandatory) is a list of commands to execute in the build step.
- `environment` (optional) defines the environment variables passed to the build step.
- `user` (optional) is a user to execute the commands under.
- `shell` (optional) overrides the [global `shell` setting](#settings).
- `working_directory` contains the path under which to mount the volume. If omitted, it is filled from the global setting.
- `forward_ssh_agent` (optional) enabled bind mounting the SSH agent socket into the build step. It defaults to `false`.
- `override_entrypoint` (optional) executes the shell as the entrypoint. It defaults to `false`.
- `mount_docker_sock` (optional) mounts `/var/run/docker.sock` into the container. It defaults to `false`.
- `forward_ssh_agent` (optional) enables mapping of the SSH agent socket into the container. It defaults to `false`.

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

If environment variables are specified without a value, the value is taken from an existing environment variable available to `insulatr`. Consider the following build definition:

```yaml
steps:
  - name: build
    environment:
      - FOO
    commands:
      - printenv
```

When it is executed using `FOO=bar insulatr`, the build step received the environment variable `FOO` with the value `bar` from the environment of `insulatr`.

## Example

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