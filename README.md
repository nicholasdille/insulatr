# insulatr

`insulatr` is a tool for container native builds

## Usage

XXX

```bash
insulatr --file insulatr.yaml
```

XXX parameters

XXX alias

```bash
alias insulatr="echo -e 'FROM nicholasdille/insulatr\nADD insulatr.yaml /' | docker image build --file - --tag insulatr:test --quiet . | xargs -r docker run -t -v /var/run/docker.sock:/var/run/docker.sock"
```

## Build definitions

XXX

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
