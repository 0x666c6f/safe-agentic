# Networking

Networking is one of the main safety controls in berth.

## Default mode

By default, each agent gets a managed Docker bridge network.
The VM setup pins each managed bridge interface to a `bt*` name so the `BERTH_EGRESS` iptables chain can apply default egress guardrails.

Intent:
- isolate agents from each other
- avoid broad reuse of the default Docker bridge
- keep the default path narrow and predictable
- block private/link-local/reserved address ranges by default while allowing normal TCP egress on 22/80/443

## Custom network

```bash
berth spawn claude --network my-net --repo ...
```

Use this only when you intentionally want different connectivity behavior.

Tradeoff:
- you gain flexibility
- you lose the managed default assumptions

## Isolated network

For especially untrusted work, use an internal Docker network:

```bash
berth vm ssh
docker network create --internal agent-isolated
exit

berth spawn claude --network agent-isolated --repo https://github.com/org/repo.git
```

## SSH forwarding and networking

`--ssh` changes auth exposure, not the network topology. It forwards an SSH agent socket into the container through the VM.

That means:
- you can use `git@...`
- the private key still stays in 1Password or your host SSH agent
- the container gains signing/auth ability, not the key material itself
