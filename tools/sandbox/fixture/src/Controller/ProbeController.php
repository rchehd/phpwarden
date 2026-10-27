<?php

namespace Drupal\phpwarden_probe\Controller;

use Drupal\Component\Serialization\Yaml;
use Symfony\Component\HttpFoundation\JsonResponse;

final class ProbeController {

  public static function page(): JsonResponse {
    $rows = \Drupal::database()->select('config', 'c')
      ->fields('c', ['name'])->range(0, 5)->execute()->fetchCol();
    return new JsonResponse([
      'drupal' => \Drupal::VERSION,
      'driver' => \Drupal::database()->driver(),
      'config_rows' => count($rows),
      'work' => self::hotspot(),
    ]);
  }

  public static function hotspot(): int {
    $size = 0;
    for ($i = 0; $i < 40; $i++) {
      $size += strlen(Yaml::encode(['fixture' => range(1, 10)]));
    }
    return $size;
  }

}
