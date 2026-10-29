<?php

namespace Drupal\phpwarden_taint_probe\Plugin\QueueWorker;

use Drupal\Core\Annotation\QueueWorker;
use Drupal\Core\Queue\QueueWorkerBase;

/** @QueueWorker(id = "phpwarden_taint_probe") */
final class ProbeQueue extends QueueWorkerBase {

  #[\Override]
  public function processItem($data): void {
    \Drupal::database()->query('SELECT name FROM users WHERE name=' . (string) $data['name']);
  }

}
