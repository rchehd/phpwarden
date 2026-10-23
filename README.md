# PHPWarden

PHPWarden runs static quality checks on Drupal projects. By default it checks only the lines you changed against a base branch. The tools run inside a Docker image, so the host needs no PHP or Composer. Results come as readable text or JSON, with stable exit codes for CI and scripts.

## Requirements

- Linux amd64, Git, and a local Docker daemon.
- Go 1.26.5 to build the CLI.
- A Drupal project on PHP 8.4. Other frameworks and PHP versions are not supported yet.

## Install

The Docker image is not published, so build it locally together with the CLI. From this repository:

```sh
go build -o bin/phpwarden ./cmd/phpwarden
docker build -f docker/Dockerfile -t phpwarden:static-php8.4-dev .
```

Rebuild both after every update: the CLI rejects an image from another version.

## Configure a project

Create `.phpwarden.yaml` in the Drupal project root:

```yaml
version: 1
profile: drupal
php:
  runtime: "8.4"
  target:
    min: "8.4"
    max: "8.4"
scope:
  base: origin/main
```

That is enough to start. PHPWarden finds the web root and custom code (`modules/custom`, `profiles/custom`, `themes/custom`) from `composer.json` and the directory layout. Core, contrib, and vendor are always excluded.

[examples/drupal.phpwarden.yaml](examples/drupal.phpwarden.yaml) shows the common settings, and [schemas/config-v1.schema.json](schemas/config-v1.schema.json) lists all of them. To check the file and see the final settings with their sources:

```sh
phpwarden config validate
phpwarden config show
```

PHPWarden looks for `.phpwarden.yaml` from the current directory up to the Git root. Use `--project PATH` or `--config PATH` to point elsewhere, for example to a Drupal project inside a monorepo.

## Run checks

```sh
phpwarden check                                # changed lines against scope.base
phpwarden check --base origin/develop          # another base branch
phpwarden check --whole-files                  # all lines of changed files
phpwarden check --mode full                    # all custom code
phpwarden check web/modules/custom/my_module   # specific paths
phpwarden check --only phpstan,phpcs           # selected checkers
phpwarden check --format json                  # JSON report
```

Changed code includes commits since the merge base, staged and unstaged edits, and new files not ignored by Git. Without `--base` or `scope.base`, PHPWarden uses `origin/HEAD`. It never fetches, so update the base branch yourself.

To see which files and lines a check would cover:

```sh
phpwarden scope
```

The project is mounted read-only and nothing is changed: PHPWarden does not fix code.

## Checkers

All checkers are enabled by default.

| ID | Checks |
|---|---|
| `debug` | Debug calls: `var_dump`, `print_r`, `dpm`, `dsm`, `kint` |
| `syntax` | PHP syntax (`php -l`) in `.php`, `.module`, `.inc`, `.install`, `.theme`, `.profile`, `.engine`, `.test` |
| `yaml` | YAML syntax |
| `whitespace` | Trailing whitespace and unresolved merge conflict markers |
| `drupal-config` | Top-level `uuid` and `_core` keys in module `config/install`, `config/optional`, `config/schema` |
| `phpcs` | PHP_CodeSniffer with Drupal Coder (`Drupal`, `DrupalPractice`); PHPCompatibility on request |
| `phpstan` | PHPStan with phpstan-drupal and deprecation rules |
| `twig-cs-fixer` | Twig template style with Twig CS Fixer |

Every checker accepts these settings under `tools.<id>`:

- `enabled`: run the checker.
- `required`: when the checker cannot run, `true` fails the whole check, `false` reports an incomplete result.
- `fail_on`: `error`, `warning`, or `never` (report without failing).
- `timeout_seconds`, `include`, `exclude`.

### PHPCS

PHPCS uses the nearest `.phpcs.xml`, `phpcs.xml`, `.phpcs.xml.dist`, or `phpcs.xml.dist` in the project. Without one it uses `Drupal` and `DrupalPractice`. Other useful settings are `standards`, `sniffs`, `exclude_sniffs`, `standard_paths` for custom standards, and `compatibility.enabled: true` for PHPCompatibility against `php.target`.

### PHPStan

PHPStan loads the project's Composer autoloader, so run `composer install` first. The installed `drupal/core` must match `composer.lock`. If it does not, PHPStan is reported as unavailable and the other checkers still run.

PHPStan uses `phpstan.neon`, `phpstan.neon.dist`, or `phpstan.dist.neon` from the project root. Without one it analyses at level 3. Other useful settings are `level`, `baseline`, `includes`, `bootstrap_files`, `deprecations`, and `memory_limit_mb`.

### Twig CS Fixer

Twig CS Fixer checks `.twig` templates. It uses `.twig-cs-fixer.php` or `.twig-cs-fixer.dist.php` from the project root. Without one it uses the default `TwigCsFixer` standard without `IncludeFunctionRule` and `CompactHashRule`, because Drupal templates use the `include` tag and long-form hashes. Other useful settings are `config`, `discover_config`, `extra_extensions`, and `extra_inputs` for files the project config loads.

Twig CS Fixer checks style, not whether a template compiles. A template it cannot read at all is reported as `File.Invalid` even outside the changed lines.

## Results

Text output lists findings and checker problems. `--format json` prints one report that follows [schemas/report-v1.schema.json](schemas/report-v1.schema.json).

| Exit code | Meaning |
|---|---|
| `0` | No blocking findings |
| `1` | Blocking findings |
| `2` | Invalid configuration, arguments, or project metadata |
| `3` | A required checker could not run, or the project changed during the check |
| `130` | Interrupted |

`PARTIAL` in text output (`complete: false` in JSON) means an optional checker could not run. `Discovery warning` lines report problems in project metadata that did not stop the check, such as duplicate keys in `composer.json` or a `vendor` that does not match `composer.lock`.
