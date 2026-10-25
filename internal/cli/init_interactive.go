package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/rchehd/phpwarden/internal/agentskills"
	"github.com/rchehd/phpwarden/internal/projectinit"
)

var initCheckDescriptions = map[string]string{
	"composer-audit":           "Package vulnerabilities; uses network access",
	"composer-require-checker": "Undeclared dependencies; full mode, standalone package",
	"composer-unused":          "Unused dependencies; full mode, standalone package",
	"composer-validate":        "Composer configuration and lock file",
	"dead-code":                "Unused PHP code; needs Composer dependencies",
	"debug":                    "Leftover debug calls",
	"deptrac":                  "Architecture boundaries; needs a project Deptrac config",
	"drupal-config":            "Drupal module configuration",
	"eslint":                   "JavaScript standards; needs project config/dependencies",
	"eventhorizon":             "Drupal caching and configuration",
	"gitleaks":                 "Exposed secrets",
	"jscpd":                    "Duplicate code",
	"phpcs":                    "PHP coding standards",
	"phpmd":                    "PHP maintainability",
	"phpstan":                  "PHP static analysis; needs Composer dependencies",
	"rector":                   "Drupal modernization; needs Composer dependencies",
	"semgrep":                  "Security patterns",
	"stylelint":                "Stylesheet standards; needs a project Stylelint config",
	"syntax":                   "PHP syntax",
	"twig-cs-fixer":            "Twig coding standards",
	"whitespace":               "Whitespace and line endings",
	"yaml":                     "YAML syntax",
}

func interactiveInit(args options, stdin io.Reader, stdout, stderr io.Writer) bool {
	if args.noInteraction || args.format != "text" || os.Getenv("CI") != "" {
		return false
	}
	terminal := func(stream any) bool {
		file, ok := stream.(*os.File)
		if !ok {
			return false
		}
		info, err := file.Stat()
		if err != nil || info.Mode()&os.ModeCharDevice == 0 {
			return false
		}
		null, err := os.Stat(os.DevNull)
		return err == nil && !os.SameFile(info, null)
	}
	return terminal(stdin) && terminal(stdout) && terminal(stderr)
}

func chooseInitChecks(ctx context.Context, stdin io.Reader, output io.Writer, defaults []projectinit.CheckChoice) ([]string, error) {
	choices := slices.Clone(defaults)
	reader, ok := stdin.(*bufio.Reader)
	if !ok {
		reader = bufio.NewReader(stdin)
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := renderInitChecks(output, choices); err != nil {
			return nil, err
		}
		answer, err := readInitAnswer(ctx, reader)
		if err != nil {
			return nil, err
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "", "done":
			selected := []string{}
			for _, choice := range choices {
				if choice.Enabled {
					selected = append(selected, choice.ID)
				}
			}
			return selected, nil
		case "q", "quit":
			return nil, context.Canceled
		case "all", "none":
			for i := range choices {
				choices[i].Enabled = strings.TrimSpace(strings.ToLower(answer)) == "all"
			}
		case "defaults":
			choices = slices.Clone(defaults)
		default:
			indexes, err := initToggleIndexes(answer, choices)
			if err != nil {
				if _, writeErr := fmt.Fprintf(output, "\n%s\n", err); writeErr != nil {
					return nil, writeErr
				}
				continue
			}
			for _, index := range indexes {
				choices[index].Enabled = !choices[index].Enabled
			}
		}
	}
}

func chooseInitAgents(ctx context.Context, reader *bufio.Reader, output io.Writer) ([]string, error) {
	for {
		if _, err := fmt.Fprint(output, "\nInstall the architecture skill for agents?\nall / codex,cursor,claude / none · Enter for none · q to cancel\n> "); err != nil {
			return nil, err
		}
		answer, err := readInitAnswer(ctx, reader)
		if err != nil {
			return nil, err
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer == "q" || answer == "quit" {
			return nil, context.Canceled
		}
		selected, err := agentskills.ParseClients(answer)
		if err == nil {
			return selected, nil
		}
		if _, err := fmt.Fprintln(output, err); err != nil {
			return nil, err
		}
	}
}

func initToggleIndexes(answer string, choices []projectinit.CheckChoice) ([]int, error) {
	indexes := []int{}
	for _, token := range strings.FieldsFunc(answer, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		number, err := strconv.Atoi(token)
		index := number - 1
		if err != nil {
			index = slices.IndexFunc(choices, func(choice projectinit.CheckChoice) bool { return choice.ID == token })
		}
		if index < 0 || index >= len(choices) {
			return nil, fmt.Errorf("Unknown check %q. Enter numbers from 1 to %d or check IDs.", token, len(choices))
		}
		if !slices.Contains(indexes, index) {
			indexes = append(indexes, index)
		}
	}
	if len(indexes) == 0 {
		return nil, fmt.Errorf("Enter check numbers, all, none, defaults, or press Enter to save")
	}
	return indexes, nil
}

func renderInitChecks(output io.Writer, choices []projectinit.CheckChoice) error {
	var text strings.Builder
	text.WriteString("\nChoose checks for this project · [x] enabled\n\n")
	enabled := 0
	for i, choice := range choices {
		mark := " "
		if choice.Enabled {
			mark = "x"
			enabled++
		}
		fmt.Fprintf(&text, "  %2d [%s] %-25s %s\n", i+1, mark, choice.ID, initCheckDescriptions[choice.ID])
	}
	fmt.Fprintf(&text, "\n%d/%d enabled. Toggle numbers or IDs (e.g. 2,5).\nall / none / defaults · Enter to save · q to cancel\n> ", enabled, len(choices))
	_, err := io.WriteString(output, text.String())
	return err
}

func readInitAnswer(ctx context.Context, reader *bufio.Reader) (string, error) {
	type answer struct {
		text string
		err  error
	}
	result := make(chan answer, 1)
	go func() {
		text, err := reader.ReadString('\n')
		result <- answer{text: text, err: err}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case line := <-result:
		if line.err == io.EOF {
			return "", context.Canceled
		}
		return strings.TrimSuffix(strings.TrimSuffix(line.text, "\n"), "\r"), line.err
	}
}
