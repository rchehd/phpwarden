The worker contains the checksum-pinned Infection 0.35.8 PHAR from the official
release. It supports the PHP 8.3/8.4 workers; older runtimes are rejected by the CLI.
The PHAR scopes its dependencies and includes the PHPUnit adapter.

`sources.json` records the PHAR and upstream source archive hashes. The worker
ships the BSD-3-Clause upstream LICENSE and all bundled LICENSE/COPYING/NOTICE
material under `/usr/local/lib/phpwarden/mutation/`. The PHAR is readable by the
non-root worker user and is invoked through PHP, never installed in the project.

Mutation testing uses fresh native PHPUnit XML/JUnit from the same private test
workspace. It does not change application images, original source or Git metadata.
