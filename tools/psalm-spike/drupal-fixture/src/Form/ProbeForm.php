<?php

namespace Drupal\phpwarden_taint_probe\Form;

use Drupal\Core\Form\FormBase;
use Drupal\Core\Form\FormStateInterface;
use Drupal\Core\Render\Markup;

final class ProbeForm extends FormBase {

  #[\Override]
  public function getFormId(): string {
    return 'phpwarden_taint_probe';
  }

  #[\Override]
  public function buildForm(array $form, FormStateInterface $form_state): array {
    return ['#markup' => Markup::create((string) $form_state->getValue('name'))];
  }

  #[\Override]
  public function submitForm(array &$form, FormStateInterface $form_state): void {}

}
