<?php

namespace Drupal\phpwarden_taint_probe;

final class InputHelper {

  public static function name(): string {
    return (string) \Drupal::request()->query->get('name');
  }

}
