package projectupdates

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var exactVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
var branchVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)
var rows = regexp.MustCompile(`(?s)<tr\b[^>]*>(.*?)</tr>`)
var cells = regexp.MustCompile(`(?s)<td\b[^>]*>(.*?)</td>`)
var tags = regexp.MustCompile(`<[^>]+>`)

func FetchHTTP(ctx context.Context, address string) ([]byte, error) {
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) > 3 || request.URL.Scheme != "https" || request.URL.Hostname() != "www.php.net" {
			return fmt.Errorf("unexpected PHP metadata redirect")
		}
		return nil
	}}
	request, err := http.NewRequestWithContext(ctx, "GET", address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "PHPWarden update assessment")
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("PHP metadata request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("PHP metadata HTTP status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if len(data) > 2<<20 {
		return nil, fmt.Errorf("PHP metadata exceeds 2 MiB")
	}
	return data, err
}

func parseSupport(data []byte) ([]Lifecycle, error) {
	result := []Lifecycle{}
	seen := map[string]bool{}
	for _, row := range rows.FindAllSubmatch(data, -1) {
		values := cells.FindAllSubmatch(row[1], -1)
		if len(values) == 0 {
			continue
		}
		clean := func(index int) string {
			return strings.TrimSpace(html.UnescapeString(tags.ReplaceAllString(string(values[index][1]), "")))
		}
		branch := clean(0)
		if !branchVersion.MatchString(branch) {
			continue
		}
		if len(values) < 6 {
			return nil, fmt.Errorf("PHP support row format is unrecognized")
		}
		active, aErr := time.Parse("2 Jan 2006", clean(3))
		security, sErr := time.Parse("2 Jan 2006", clean(5))
		if aErr != nil || sErr != nil || !security.After(active) || seen[branch] {
			return nil, fmt.Errorf("invalid PHP support metadata")
		}
		seen[branch] = true
		result = append(result, Lifecycle{Branch: branch, ActiveUntil: active.Format("2006-01-02"), SecurityUntil: security.Format("2006-01-02")})
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("PHP support table format is unrecognized")
	}
	slices.SortFunc(result, func(a, b Lifecycle) int { return compareVersion(a.Branch+".0", b.Branch+".0") })
	return result, nil
}

func compareVersion(a, b string) int {
	x, y := strings.Split(a, "."), strings.Split(b, ".")
	for i := range 3 {
		left, _ := strconv.ParseUint(x[i], 10, 32)
		right, _ := strconv.ParseUint(y[i], 10, 32)
		if left < right {
			return -1
		}
		if left > right {
			return 1
		}
	}
	return 0
}

type release struct {
	Tags []string `json:"tags"`
	Date string   `json:"date"`
}

func parseRelease(data []byte, branch string) (string, release, error) {
	var values map[string]release
	if err := json.Unmarshal(data, &values); err != nil || len(values) != 1 {
		return "", release{}, fmt.Errorf("invalid PHP release response")
	}
	for version, value := range values {
		if !exactVersion.MatchString(version) || !strings.HasPrefix(version, branch+".") || value.Date == "" {
			return "", release{}, fmt.Errorf("PHP release does not match requested branch")
		}
		return version, value, nil
	}
	return "", release{}, fmt.Errorf("missing PHP release")
}

func phpMetadata(ctx context.Context, request Request, result *Report) {
	result.PHP = PHP{Branch: request.Branch, Support: "unknown", SupportedBranches: []Lifecycle{}, Note: "Application patch version is unknown. Composer platform and worker PHP do not establish it. Distributor backports require vendor-specific verification."}
	if request.Options.RuntimeVersion != "" {
		result.PHP.Current = &request.Options.RuntimeVersion
		result.PHP.Note = "Application version supplied explicitly; distributor backports are not assessed."
	}
	get := func(id, address string, validate func([]byte) error) []byte {
		data, source, err := cached(ctx, id, address, request.Settings, request.Now, validate, func(ctx context.Context) ([]byte, error) { return request.HTTP(ctx, address) })
		if err != nil {
			source.Note = err.Error()
			result.Complete = false
		}
		if source.Status == "stale" {
			result.Complete = false
		}
		result.Sources = append(result.Sources, source)
		return data
	}
	address := "https://www.php.net/releases/index.php?json=1&max=1&version=" + url.QueryEscape(request.Branch)
	data := get("php_releases", address, func(data []byte) error { _, _, err := parseRelease(data, request.Branch); return err })
	if data != nil {
		version, value, _ := parseRelease(data, request.Branch)
		result.PHP.Latest, result.PHP.SecurityRelease = version, slices.Contains(value.Tags, "security")
		if result.PHP.Current != nil {
			available := compareVersion(*result.PHP.Current, version) < 0
			result.PHP.UpdateAvailable = &available
		}
	}
	support := get("php_support", "https://www.php.net/supported-versions.php", func(data []byte) error { _, err := parseSupport(data); return err })
	if support == nil {
		return
	}
	branches, _ := parseSupport(support)
	result.PHP.SupportedBranches = branches
	for _, branch := range branches {
		if branch.Branch == request.Branch {
			result.PHP.Lifecycle = &branch
			day := request.Now.UTC().Format("2006-01-02")
			switch {
			case day > branch.SecurityUntil:
				result.PHP.Support = "eol"
			case day > branch.ActiveUntil:
				result.PHP.Support = "security"
			default:
				result.PHP.Support = "active"
			}
			return
		}
	}
	if result.PHP.Latest != "" && compareVersion(request.Branch+".0", branches[0].Branch+".0") < 0 {
		result.PHP.Support = "eol"
	} else {
		result.Complete = false
		result.Notes = append(result.Notes, "PHP branch support could not be established.")
	}
}
