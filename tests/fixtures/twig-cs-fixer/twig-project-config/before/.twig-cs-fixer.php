<?php

$ruleset = new TwigCsFixer\Ruleset\Ruleset();
$ruleset->addRule(new TwigCsFixer\Rules\Function\IncludeFunctionRule());
$config = new TwigCsFixer\Config\Config();
$config->setRuleset($ruleset);

return $config;
