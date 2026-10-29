<?php

namespace Drupal\phpwarden_taint_probe\Controller;

use Drupal\Component\Utility\Html;
use Drupal\Core\Render\Markup;
use Drupal\phpwarden_taint_probe\InputHelper;

final class ProbeController {

  public function trusted(): array {
    return ['#markup' => Markup::create(InputHelper::name())];
  }

  public function escaped(): array {
    return ['#markup' => Markup::create(Html::escape(InputHelper::name()))];
  }

  public function plain(): array {
    return ['#plain_text' => InputHelper::name()];
  }

  public function filtered(): array {
    return ['#markup' => InputHelper::name()];
  }

  public function template(): array {
    return ['#type' => 'inline_template', '#template' => InputHelper::name()];
  }

  public function query(): array {
    \Drupal::database()->query('SELECT name FROM users WHERE name=' . InputHelper::name());
    return [];
  }

  public function bound(): array {
    \Drupal::database()->query(
      'SELECT name FROM users WHERE name=:name',
      [':name' => InputHelper::name()],
    );
    return [];
  }

  public function routeValue(string $name): array {
    echo $name;
    return [];
  }

}
