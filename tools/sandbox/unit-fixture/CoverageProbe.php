<?php

namespace Drupal\phpwarden_probe;

final class CoverageProbe {

  public static function choose(bool $positive): int {
    if ($positive) {
      return 1;
    }
    return -1;
  }

}
