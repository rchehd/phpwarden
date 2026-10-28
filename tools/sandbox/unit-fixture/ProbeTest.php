<?php

namespace Drupal\Tests\phpwarden_probe\Unit;

use Drupal\phpwarden_probe\Controller\ProbeController;
use Drupal\Tests\UnitTestCase;

final class ProbeTest extends UnitTestCase {

  public function testHotspot(): void {
    self::assertGreaterThan(0, ProbeController::hotspot());
  }

  public function testWorkspaceIsWritable(): void {
    self::assertNotFalse(file_put_contents(DRUPAL_ROOT . '/phpwarden-unit-probe', 'isolated'));
  }

}
