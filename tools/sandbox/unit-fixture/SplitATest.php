<?php

namespace Drupal\Tests\phpwarden_probe\Unit;

use Drupal\phpwarden_probe\CoverageProbe;
use Drupal\Tests\UnitTestCase;

final class SplitATest extends UnitTestCase {

  public function testPositive(): void {
    self::assertSame(1, CoverageProbe::choose(TRUE));
  }

}
