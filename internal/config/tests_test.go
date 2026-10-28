package config

import (
	"strings"
	"testing"
)

func TestProjectTestsSettingsAndValidation(t *testing.T) {
	base := "version: 1\nprofile: drupal\nphp: {runtime: '8.3', target: {min: '8.3', max: '8.3'}}\n"
	value, err := Parse([]byte(base))
	if err != nil || value.Config.Tests.PHPUnit != "auto" || value.Config.Tests.Config != "auto" || value.Config.Tests.TimeoutSeconds != 300 || value.Config.Tests.Paths == nil {
		t.Fatalf("%+v %v", value.Config.Tests, err)
	}
	for _, extra := range []string{
		"tests: {timeout_seconds: 0}", "tests: {timeout_seconds: 3601}",
		"tests: {paths: [../escape]}", "tests: {phpunit: /outside/phpunit}", "tests: {config: ../phpunit.xml}",
		"tests: {paths: [tests], suite: unit}", "tests: {unknown: true}",
	} {
		if _, err := Parse([]byte(base + extra + "\n")); err == nil {
			t.Fatalf("accepted %s", extra)
		}
	}
	value, err = Parse([]byte(base + "tests: {config: backend/phpunit.xml, paths: [backend/tests], timeout_seconds: 900}\n"))
	if err != nil || value.Config.Tests.TimeoutSeconds != 900 || !strings.Contains(value.Config.Tests.Config, "backend/") {
		t.Fatalf("%+v %v", value.Config.Tests, err)
	}
}
