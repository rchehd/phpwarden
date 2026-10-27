package drupal

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/projectupdates"
)

type releaseTerm struct {
	Name  string `xml:"name"`
	Value string `xml:"value"`
}
type projectRelease struct {
	Version  string        `xml:"version"`
	Status   string        `xml:"status"`
	Link     string        `xml:"release_link"`
	Terms    []releaseTerm `xml:"terms>term"`
	Security struct {
		Covered string `xml:"covered,attr"`
	} `xml:"security"`
}
type releaseHistory struct {
	XMLName  xml.Name         `xml:"project"`
	ID       string           `xml:"short_name"`
	Status   string           `xml:"project_status"`
	Branches string           `xml:"supported_branches"`
	Releases []projectRelease `xml:"releases>release"`
}

func parseReleaseHistory(data []byte, id string) (releaseHistory, error) {
	var history releaseHistory
	decoder := xml.NewDecoder(bytes.NewReader(data))
	if len(data) > 8<<20 || decoder.Decode(&history) != nil || history.ID != id {
		return history, fmt.Errorf("invalid Drupal release history for %s", id)
	}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return history, fmt.Errorf("trailing Drupal release history data")
		}
		if text, ok := token.(xml.CharData); ok && len(bytes.TrimSpace(text)) == 0 {
			continue
		}
		if _, ok := token.(xml.Comment); ok {
			continue
		}
		return history, fmt.Errorf("trailing Drupal release history data")
	}
	if !slices.Contains([]string{"published", "unsupported", "unpublished", "revoked", "insecure"}, history.Status) {
		return history, fmt.Errorf("unknown Drupal project status")
	}
	seen := map[string]bool{}
	for _, item := range history.Releases {
		if item.Version == "" || seen[item.Version] || !slices.Contains([]string{"published", "unpublished", "revoked"}, item.Status) || !slices.Contains([]string{"", "0", "1"}, item.Security.Covered) {
			return history, fmt.Errorf("invalid or duplicate Drupal release")
		}
		seen[item.Version] = true
	}
	return history, nil
}

func fetchReleaseHistory(ctx context.Context, id string) ([]byte, error) {
	client := http.Client{Timeout: 30 * time.Second, CheckRedirect: func(request *http.Request, previous []*http.Request) error {
		if len(previous) > 3 || request.URL.Scheme != "https" || request.URL.Host != "updates.drupal.org" {
			return fmt.Errorf("unexpected Drupal metadata redirect")
		}
		return nil
	}}
	request, err := http.NewRequestWithContext(ctx, "GET", "https://updates.drupal.org/release-history/"+id+"/current", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "PHPWarden release assessment")
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Drupal release metadata request failed for %s", id)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Drupal release metadata HTTP %d for %s", response.StatusCode, id)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if len(data) > 8<<20 {
		return nil, fmt.Errorf("Drupal release history exceeds 8 MiB")
	}
	return data, err
}

func assessRelease(item *projectupdates.Project, history releaseHistory) bool {
	setStatus := func(status, reason string) { item.Status = status; item.Reasons = append(item.Reasons, reason) }
	switch history.Status {
	case "revoked", "unpublished":
		setStatus("revoked", "Drupal.org marks this project as revoked/unpublished.")
		return true
	case "insecure":
		setStatus("insecure", "Drupal.org marks this project as insecure.")
		return true
	case "unsupported":
		setStatus("unsupported", "Drupal.org marks this project as unsupported.")
		return true
	}
	if item.Current == "" || strings.Contains(item.Current, "dev") {
		setStatus("unknown", "No exact packaged release version; development snapshots cannot be assessed as a tagged release.")
		return false
	}
	var installed *projectRelease
	for i := range history.Releases {
		if releaseIdentity(history.Releases[i].Version) == releaseIdentity(item.Current) {
			if installed != nil {
				setStatus("unknown", "Multiple release tags map to this version.")
				return false
			}
			installed = &history.Releases[i]
		}
	}
	if installed == nil {
		setStatus("unknown", "Exact version is absent from release history; this does not establish revocation.")
		return false
	}
	item.ReleaseURL = installed.Link
	if installed.Security.Covered != "" {
		covered := installed.Security.Covered == "1"
		item.SecurityCovered = &covered
	}
	if installed.Status != "published" {
		setStatus("revoked", "The exact release is unpublished/revoked.")
		return true
	}
	for _, term := range installed.Terms {
		if term.Name == "Release type" && term.Value == "Insecure" {
			setStatus("insecure", "Drupal.org marks the exact release as insecure.")
			return true
		}
	}
	for _, term := range installed.Terms {
		if term.Name == "Release type" && term.Value == "Unsupported" {
			setStatus("unsupported", "Drupal.org marks the exact release as unsupported.")
			return true
		}
	}
	if strings.TrimSpace(history.Branches) == "" {
		setStatus("unknown", "Supported release branches are absent from the feed.")
		return false
	}
	for _, branch := range strings.Split(history.Branches, ",") {
		if branch = strings.TrimSpace(branch); branch != "" && strings.HasPrefix(installed.Version, branch) {
			item.Status = "supported"
			return true
		}
	}
	setStatus("unsupported", "The exact release is outside Drupal.org's supported branches.")
	return true
}

func UpdateAssessment(directory, composerRoot string, facts *profile.Facts) projectupdates.ProfileFetcher {
	return releaseAssessment(directory, composerRoot, facts, fetchReleaseHistory)
}

func releaseAssessment(directory, composerRoot string, facts *profile.Facts, fetch func(context.Context, string) ([]byte, error)) projectupdates.ProfileFetcher {
	return func(ctx context.Context, metadata projectupdates.Metadata) projectupdates.ProfileReport {
		result := projectupdates.ProfileReport{Complete: true, Projects: []projectupdates.Project{}}
		items, fingerprint, notes, err := updateInventory(directory, composerRoot, facts)
		result.Notes = append(result.Notes, notes...)
		if err != nil || len(items) == 0 {
			result.Complete = false
			message := "No identifiable Drupal.org projects were found."
			if err != nil {
				message = err.Error()
			}
			result.Sources = append(result.Sources, projectupdates.Source{ID: "drupal_inventory", Status: "unavailable", Note: message})
			return result
		}
		ids := []string{}
		for _, item := range items {
			if !slices.Contains(ids, item.ID) {
				ids = append(ids, item.ID)
			}
		}
		type reply struct {
			history releaseHistory
			source  projectupdates.Source
			err     error
		}
		replies := make([]reply, len(ids))
		jobs := make(chan int, len(ids))
		for i := range ids {
			jobs <- i
		}
		close(jobs)
		var workers sync.WaitGroup
		for range min(4, len(ids)) {
			workers.Go(func() {
				for i := range jobs {
					id := ids[i]
					validate := func(data []byte) error { _, err := parseReleaseHistory(data, id); return err }
					data, source, err := metadata(ctx, "drupal/"+id, "release-history-current-v1/"+id, validate, func(ctx context.Context) ([]byte, error) { return fetch(ctx, id) })
					replies[i] = reply{source: source, err: err}
					if err == nil {
						replies[i].history, replies[i].err = parseReleaseHistory(data, id)
					}
				}
			})
		}
		workers.Wait()
		for i, id := range ids {
			reply := replies[i]
			if reply.err != nil {
				reply.source.Note = reply.err.Error()
			}
			result.Sources = append(result.Sources, reply.source)
			if reply.err != nil || reply.source.Status == "stale" {
				result.Complete = false
			}
			for _, item := range items {
				if item.ID != id {
					continue
				}
				if reply.err != nil {
					item.Reasons = append(item.Reasons, "Drupal release history is unavailable.")
				} else if !assessRelease(&item, reply.history) {
					result.Complete = false
				}
				result.Projects = append(result.Projects, item)
			}
		}
		_, current, _, err := updateInventory(directory, composerRoot, facts)
		if err != nil || current != fingerprint {
			result.Complete = false
			result.Notes = append(result.Notes, "Drupal inventory changed during release assessment; rerun it.")
		}
		return result
	}
}
