# Development and distribution

Development validation includes installed Drupal 10/11 sandboxes using Compose, DDEV, bare Docker and host PHP in the
**Sandbox** workflow on relevant pull requests, on manual runs and before worker publication in **Build**. See [the
sandbox suite](../tools/sandbox/README.md). Third-party notices are available with
`phpwarden --licenses`; see [dependency material](../tools/spx/LICENSES.md).

### Build from source

Build the CLI and the Docker images locally. There is one image per PHP version, each with the same checker versions. From this repository:

```sh
go build -o bin/phpwarden ./cmd/phpwarden
docker buildx bake         # all four images
docker buildx bake php83   # only PHP 8.3
```

Instead of building the images, you can pull the development images published by a manual CI run from `main` and give them the local names the CLI expects:

```sh
for v in 8.1 8.2 8.3 8.4; do
  docker pull ghcr.io/rchehd/phpwarden:static-php$v-dev
  docker tag ghcr.io/rchehd/phpwarden:static-php$v-dev phpwarden:static-php$v-dev
done
```

Rebuild or pull again after every update: the CLI rejects an image from another version.

The development toolset revision is defined once in `docker-bake.json` (`PHPWARDEN_TOOLSET`). The CLI embeds that file, development manifest entries derive their IDs from it, and Docker Bake passes the matching ID to the image label. Change this value when worker/toolset compatibility changes, then rebuild the CLI and images together. Published release manifests still specify explicit toolset IDs.

The images are ready-to-run PHPWarden installations: they contain the same CLI binary, the worker, PHP and the analyzers. Their PHP version tags select the execution runtime. Docker's intermediate build stages are not published.

There are two ways to run a check:

| Installation | How it runs |
|---|---|
| Standalone CLI | The host binary reads config, downloads a missing published worker image when needed, and launches it |
| Docker image directly | The image runs `phpwarden worker check` inside the mounted project |

For example, with a Git project configured for PHP 8.4 and its dependencies installed, run from that project's Git root:

```sh
docker run --rm -v "$PWD:/workspace:ro" ghcr.io/rchehd/phpwarden:static-php8.4-dev worker check --commits 2
```

Without command arguments, the image defaults to `worker check`; include `worker check` explicitly when passing options as above. Image-only execution selects the PHP runtime through the image tag; the standalone CLI selects it from `.phpwarden.yaml`.

### CI builds and distribution

The **Checks** workflow runs automatically on branch pushes and pull requests,
selecting Go/native and Python/fixture jobs from changed files. Documentation-only
PRs skip those jobs; the stable **Checks complete** job still reports success.
Manual runs and reusable calls from Build or Mutation tests run the full suite.

**Sandbox** and **Mutation tests** also run automatically on relevant pull requests.
Sandbox selects installed Drupal application or Docker-less acceptance based on
runtime, installation and harness changes. Mutation tests select test-runner,
configuration, source-scope and mutation-harness changes. Worker images, toolset
metadata, bundled dependencies and shared CI filtering trigger their consumers.
An isolated Go test change selects Go checks; a Python unit-test change selects
Python checks. Embedded Markdown/templates and text fixtures remain code/test inputs.

The shared selector is `.github/actions/changes` with path rules in
`tools/ci/changes.py`. It compares PR merge-base to head and the complete before/after
range for pushes, including deleted and renamed paths. A new branch with no previous
commit or unavailable comparison history selects all groups conservatively.
Manual Sandbox selections remain available, and manual Mutation tests run the full
Drupal 10/11 × PHP 8.3/8.4 matrix. PR acceptance builds temporary workers for tests;
publication stays in Build. Superseded PR validation runs are cancelled.

The separate **Build** workflow runs manually or on a pushed version tag such as `v0.1.0-rc.1`. It calls Checks for the same commit and builds only after those checks pass. Open the [Build workflow page](https://github.com/rchehd/phpwarden/actions/workflows/build.yml), choose **Run workflow**, select `main`, and choose a target:

| Target | Result |
|---|---|
| `all` (default) | CLI/project packages, all PHP 8.1–8.4 workers and agent plugin packages |
| `cli` | CLI archive and Composer/npm packages |
| `images` | Worker images and their Docker acceptance tests only |
| `plugins` | Three client plugin ZIPs, a catalog ZIP and checksums; no CLI or image build |

Enable `publish_images` with `all` or `images` on `main` to publish the tested development images to GHCR. Without it, images are built and tested without publication. Version-tag pushes always build CLI, images and plugins, publish version-specific image tags such as `static-php8.4-v0.1.0-rc.1` without moving the floating development tags, and publish a GitHub Release with all matching distribution assets and combined checksums after successful checks and builds. Tags with a prerelease suffix create prereleases; plain `update` selects stable releases.

To request just the CLI archive with GitHub CLI:

```sh
gh workflow run build.yml --repo rchehd/phpwarden --ref main -f target=cli
```

Use `-f target=all -f publish_images=true` to build both targets and publish the images.

Build runs that include the CLI produce a `phpwarden-linux-amd64-<commit>` Actions artifact containing `phpwarden-linux-amd64.tar.gz` and `SHA256SUMS`. Extract the downloaded artifact, run `sha256sum --ignore-missing -c SHA256SUMS`, then extract the tarball. It contains the executable, README, source commit SHA and `release.json` build identity.

With `all` + `publish_images=true`, the image job records real published digests, then the CLI job embeds that manifest. This development snapshot has a `v0.0.0-dev.g<commit>` version, supports automatic worker downloads, and can later update to a published stable CLI. The `g` keeps even an all-numeric commit prefix valid under SemVer. The archive includes `compatibility.json`, also uploaded separately. The same artifact also includes checksum-listed Composer/npm packages and four Compose integration archives. CLI-only or unpublished builds produce the development CLI archive and Composer/npm packages, without Compose archives or a published compatibility manifest. `internal/toolset/publication-v1.json` declares framework ranges: Drupal 10.0–10.6 on PHP 8.1–8.4 and Drupal 11.0–11.4 on PHP 8.3–8.4. Project Composer requirements can further restrict a combination. The ranges describe declared compatibility, not proof of every application/tool combination.

Local distribution packaging requires a fresh destination for generated assets.
It refuses existing archives, executables and checksums before overwriting them;
an explicitly supplied input manifest can already be in that directory. Use a
new directory for each version so stale Compose bundles cannot enter a new build.

CLI-only builds and builds without image publication use source/development compatibility and require locally prepared images. They do not claim release compatibility or support self-replacement. `--version` reports the embedded version and commit; `--build-info` provides JSON including installation type and, when available, manifest checksum. GHCR contains worker images, while CLI archives are available through Actions artifacts and versioned GitHub Releases. Composer/npm package names are not yet registered in public package registries.
