# insulatr

[Documentation](https://godoc.org/github.com/docker/docker/client)

[Examples](https://docs.docker.com/develop/sdk/examples/)

[GitLab Runner Docker Executor](https://gitlab.com/gitlab-org/gitlab-runner/blob/master/executors/docker/executor_docker.go#L1038)

[Docker CLI](https://github.com/docker/cli/blob/master/cli/command/container/run.go#L268)

[Command line arguments](https://github.com/mkideal/cli)

## TODO

### Phase 0

- [X] Get return code from commands in containers
- [X] Stop execution on error but include cleanup
- Parameters
  - [X] Add parameter for file
  - [X] Add parameter for volume and network reuse
  - [X] Add parameter for volume and network cleanup
- [X] Improve functions to cover services, checkout and steps
  - steps: blocking, stdin, volume, network, live log
  - services: background, network, log
  - clone: blocking, stdin, volume, live log
- [X] Add logging for services
  - Log to console on stop
- [X] Sanity checks for data structures
- [X] Improve key names in YAML files
- [X] Implement services start
- [X] Implement services stop
- [X] Improve output
- [ ] Fix merging of stdout and stderr

### Phase 1

- [ ] Add logging to file (in addition to console)
  - Use [TeeReader](https://golang.org/pkg/io/#TeeReader)
- [X] Allow overriding entrypoint instead of command
- [X] Add user
- [ ] Inject local files (repo definition without location)
  - `sh -c 'tar -cz . | docker run --interactive --rm --volume $VolumeMount --workdir $ContainerWorkingDirectory alpine tar -xz'`
  - [discussion](https://github.com/moby/moby/issues/26652)
  - [example code](https://github.com/docker/cli/blob/b1d27091e50595fecd8a2a4429557b70681395b2/cli/command/container/cp.go#L249)
- [ ] Import environment variables (name without assignment)
- [ ] Add global environments (top level)
- [ ] git authentication
- [ ] SSH git
- [X] Support checkout from branches/tags/commits
  - branches: git checkout BRANCH
  - tags: git fetch --tags && git checkout TAG
  - commit: git checkout COMMIT
- [ ] Support checkout from git references?
  - ref: git fetch origin +refs/pull/*:refs/remotes/origin/pr/* && git checkout -b NAME origin/pr/N/head
- [X] git shallow clone
- [ ] git annotations?
