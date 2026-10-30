# PHP 8.4 execution gate

On 2026-10-30 the pinned Composer bundle (Psalm 6.20.3, 69 unchanged packages,
installed with PHP 8.3) was executed on PHP **8.4.27** in an existing isolated
container. The ten common native XML/baseline cases passed. The added unsafe
property-hook control returned no taint findings, so the gate failed as intended.
The escaped-hook case was then checked separately as part of a minimal comparison.

This does not establish installation compatibility on PHP 8.4 or support for the
complete Drupal profile. The worker still rejects PHP 8.4 explicitly; its supported
candidate runtime remains PHP 8.3.16+ in the 8.3 series.

| Minimal control | Actual PHP output for `<em>marker</em>` | Ordinary Psalm | Taint Psalm |
| --- | --- | --- | --- |
| Ordinary method returning request input | Raw HTML | `[]`, exit 0 | `TaintedHtml`, `TaintedTextWithQuotes`, exit 2 |
| Getter hook returning the same input | Raw HTML | `[]`, exit 0 | `[]`, exit 0: false negative |
| Getter hook escaping the input | Escaped HTML | `[]`, exit 0 | `[]`, exit 0 |

The comparison used `phpVersion="8.4"` explicitly and no Drupal plugin, baseline,
issue suppression or PHPWarden model plugin. This isolates the observed failure
from Drupal preparation and native configuration overlays. It does not claim all
property-hook flows fail or that other Psalm versions have the same result.

## Minimal example

Place this in `src/HookedInput.php` in a disposable directory:

```php
<?php
final class HookedInput {
    public string $value { get => (string) ($_GET['name'] ?? ''); }
}
```

In `src/Entry.php`:

```php
<?php
require_once __DIR__ . '/HookedInput.php';
echo (new HookedInput())->value;
```

Use this `psalm.xml` and run the pinned tool with PHP 8.4:

```xml
<psalm errorLevel="6" phpVersion="8.4" resolveFromConfigFile="true" cacheDirectory="/tmp/psalm-cache">
  <projectFiles><directory name="src"/></projectFiles>
</psalm>
```

```sh
php8.4 /scratch/psalm/vendor/bin/psalm --config=psalm.xml --no-cache --no-diff --no-progress --threads=1 --scan-threads=1 --show-info=false --output-format=json
php8.4 /scratch/psalm/vendor/bin/psalm --config=psalm.xml --taint-analysis --no-cache --no-diff --no-progress --threads=1 --scan-threads=1 --show-info=false --output-format=json
```

Both commands returned `[]`. For the ordinary-method control, replace the hook
with `public function value(): string { return (string) ($_GET['name'] ?? ''); }`
and call `value()` in `Entry.php`. That control produced the two expected taint
types. For the safe getter, wrap the request value with
`htmlspecialchars(..., ENT_QUOTES, 'UTF-8')`.

The complete native harness includes the unsafe/safe hook pair when measured PHP
is 8.4. It keeps the unsafe assertion expecting findings; the false negative is
not accepted as a passing result. The first unsafe failure stops the suite.
Successful ordinary analysis alone must not remove the public runtime guard.
Any future engine/model change needs this regression, the complete native cases
and the Drupal 10/11 worker matrix before expanding advertised support.
