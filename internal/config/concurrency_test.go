package config

import (
	"strconv"
	"testing"
)

func TestRunnerConcurrency(t *testing.T) {
	for _, value := range []string{"", "0", "1", "4", "128"} {
		t.Run("value_"+value, func(t *testing.T) {
			data := minimal
			if value != "" {
				data += "runner: {concurrency: " + value + "}\n"
			}
			effective, err := Parse([]byte(data))
			if err != nil {
				t.Fatal(err)
			}
			want, source := 1, "core_default"
			if value != "" {
				want, _ = strconv.Atoi(value)
				source = "project_config"
			}
			if effective.Config.Runner.Concurrency != want || effective.Provenance["runner.concurrency"] != source {
				t.Fatalf("concurrency/provenance: %+v, %s", effective.Config.Runner, effective.Provenance["runner.concurrency"])
			}
		})
	}
	for _, value := range []string{"-1", "1.5", "true", "\"all\""} {
		if _, err := Parse([]byte(minimal + "runner: {concurrency: " + value + "}\n")); err == nil {
			t.Fatalf("invalid concurrency %s accepted", value)
		}
	}
}
