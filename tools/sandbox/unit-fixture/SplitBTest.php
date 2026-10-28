<?php

namespace Drupal\Tests\phpwarden_probe\Unit;

use Drupal\phpwarden_probe\CoverageProbe;
use Drupal\Tests\UnitTestCase;

final class SplitBTest extends UnitTestCase {

  public function testNegative(): void {
    self::assertSame(-1, CoverageProbe::choose(FALSE));
  }

}
