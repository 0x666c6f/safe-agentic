# Threat Surface

Each opt-in flag changes the trust boundary.

## `--ssh`

Adds:
- ability to authenticate to SSH remotes your agent can reach
- ability to push to repos your SSH identity can write to

Does not add:
- raw access to the private key material

## `--reuse-auth`

Adds:
- persistence of Claude/Codex auth across sessions

Tradeoff:
- another session reusing that auth volume can read the same token state

## `--reuse-gh-auth`

Adds:
- persistent GitHub CLI auth across sessions

Tradeoff:
- shared token state in the auth volume

## `--aws <profile>`

Adds:
- AWS API access for whatever the chosen profile can do

Tradeoff:
- the agent can use those credentials for the life of the session

## `--docker`

Adds:
- Docker-in-Docker sidecar access

Tradeoff:
- broader runtime capability inside the session

## `--docker-socket`

Adds:
- direct control of the VM Docker daemon

Tradeoff:
- much broader blast radius than `--docker`

## `--network <name>`

Adds:
- custom network topology

Tradeoff:
- you leave the managed default path and own the network consequences yourself
