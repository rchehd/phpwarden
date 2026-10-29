<?php

namespace PHPWarden\PsalmSpike;

use Psalm\Plugin\PluginEntryPointInterface;
use Psalm\Plugin\RegistrationInterface;
use SimpleXMLElement;

final class Plugin implements PluginEntryPointInterface
{
    public function __invoke(RegistrationInterface $registration, ?SimpleXMLElement $config = null): void
    {
        $registration->addStubFile(__DIR__ . '/form-state.php');
        $registration->addStubFile(__DIR__ . '/database.php');
    }
}
