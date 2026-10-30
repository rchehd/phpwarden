# PHPWarden shared plugin

The three native plugin manifests load the same nine packages from `skills/`.
Repository catalogs resolve this directory from the repository root. Skills and
their supporting resources have one committed source, also embedded in the CLI
for `init --agents` and in generated plugin archives.

Git catalogs are for development and omit a fixed plugin version. Versioned
archives carry the same version and exact source commit as their CLI build.
Client update behavior still needs live verification. See
[plugin setup](../../plugins/README.md) for installation routes and limitations.

The plugin supplies skills. Install the PHPWarden CLI separately; project
instructions, completion hooks and performance MCP require separate setup.

The `phpwarden-setup` skill includes a shell bootstrap and a generated copy of the
canonical root `install.sh`. Regenerate source metadata/installer with
`phpwarden-dist catalogs`; the drift test prevents maintaining two implementations.
Codex onboarding targets this skill; other clients invoke it explicitly. Status
is read-only and installation requires a setup/install request.
