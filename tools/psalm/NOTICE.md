# Psalm development bundle notices

This isolated bundle uses the exact dependencies in `composer.lock`. Package
license declarations, revisions and SHA256 hashes of the retained notice files
are recorded in the generated `inventory.json`. Original package notices are
copied without modification under `notices/`; executable PHP sources stay under
`vendor/`. The source archives listed in `sources.json` are retained under
`sources/` with their verified checksums.

The Drupal plugin at revision `073e7a4d5fc159fbf47decf16296edd7f8a545d1`
declares `LGPL-2.1-only` in Composer metadata, but its actual `LICENSE` adds
the **Commons Clause License Condition v1.0**. That condition restricts selling
the software as defined in the notice. Keep the original condition with every
copy; the Composer license field alone does not describe this dependency.
The unmodified pinned source archive is included alongside that notice.
The referenced full LGPL 2.1 text is retained under `licenses/LGPL-2.1.txt`.

JsonMapper 5.0.1 declares OSL-3.0; its original license and pinned source archive
are also included. Other package notices remain individually attributed in the
inventory. These dependencies do not select a license for PHPWarden itself.

Public image integration is pending a distribution decision about the Drupal
plugin's additional condition. Generating these notices does not authorize or
enable publication. The current worker images and compatibility capabilities
remain unchanged by this development preparation command.
