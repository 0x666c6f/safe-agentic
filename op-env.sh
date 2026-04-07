# 1Password secret references for optional use with `op run --env-file`.
# Not needed for basic operation (Claude/Codex use OAuth).
# Useful for injecting AWS credentials or other secrets into containers.
#
# Usage: op run --env-file=op-env.sh -- <command>
#
# AWS_ACCESS_KEY_ID=op://vault/item/field
# AWS_SECRET_ACCESS_KEY=op://vault/item/field
