<?php

namespace Drupal\phpwarden_probe;

abstract class UncoveredProbe {

  abstract public function contract(): void;

  public static function neverCalled(bool $positive): int {
    if ($positive) {
      return 1;
    }
    return -1;
  }

}
