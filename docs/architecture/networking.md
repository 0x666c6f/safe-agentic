# Networking

Networking is one of the main safety controls in safe-agentic.

## Default mode

By default, each agent gets a managed Docker bridge network.

Intent:
- isolate agents from each other
- avoid broad reuse of the default Docker bridge
- keep the default path narrow and predictable

## Custom network

```bash
safe-ag spawn claude --network my-net --repo ...
```

Use this only when you intentionally want different connectivity behavior.

Tradeoff:
- you gain flexibility
- you lose the managed default assumptions

## Isolated network

For especially untrusted work, use an internal Docker network:

```bash
safe-ag vm ssh
docker network create --internal agent-isolated
exit

safe-ag spawn claude --network agent-isolated --repo https://github.com/org/repo.git
```

## SSH forwarding and networking

`--ssh` changes auth exposure, not the network topology. It forwards an SSH agent socket into the container through the VM.

That means:
- you can use `git@...`
- the private key still stays in 1Password or your host SSH agent
- the container gains signing/auth ability, not the key material itself
