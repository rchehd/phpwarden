package projectupdates

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func ValidateRuntime(version, branch string) error {
	if version != "" && (!exactVersion.MatchString(version) || !strings.HasPrefix(version, branch+".")) {
		return fmt.Errorf("--runtime-version must be X.Y.Z in configured PHP branch %s", branch)
	}
	return nil
}

func Run(ctx context.Context, request Request) Report {
	result := Report{Version: 1, Command: "updates", Complete: true, Packages: []Package{}, Projects: []Project{}, Sources: []Source{}, Notes: []string{"Coverage: Composer-managed locked packages, profile-specific project release status and upstream PHP release/support metadata. Vendor backports are not assessed. Available versions are candidates; application PHP/extension compatibility is not assessed, and no full dependency solve or upgrade is performed."}}
	if request.Now.IsZero() {
		request.Now = time.Now()
	}
	if request.HTTP == nil {
		request.HTTP = FetchHTTP
	}
	if err := ValidateRuntime(request.Options.RuntimeVersion, request.Branch); err != nil {
		result.ExitCode = 2
		result.Complete = false
		result.Notes = append(result.Notes, err.Error())
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(request.Settings.TimeoutSeconds)*time.Second)
	defer cancel()
	phpMetadata(ctx, request, &result)
	if request.Profile != nil {
		metadata := func(ctx context.Context, id, identity string, validate func([]byte) error, fetch func(context.Context) ([]byte, error)) ([]byte, Source, error) {
			return cached(ctx, id, identity, request.Settings, request.Now, validate, fetch)
		}
		assessment := request.Profile(ctx, metadata)
		result.Complete = result.Complete && assessment.Complete
		result.Sources = append(result.Sources, assessment.Sources...)
		result.Notes = append(result.Notes, assessment.Notes...)
		for _, item := range assessment.Projects {
			if request.Options.Direct && !item.Direct || request.Options.NoDev && item.Dev {
				continue
			}
			if request.Options.Security && item.Status != "revoked" && item.Status != "unsupported" && item.Status != "insecure" && (item.SecurityCovered == nil || *item.SecurityCovered) {
				continue
			}
			result.Projects = append(result.Projects, item)
		}
	}
	root, err := os.OpenRoot(request.Location.Root)
	var composer, lock []byte
	if err == nil {
		defer root.Close()
		composer, err = readProjectFile(root, filepath.Join(request.ComposerRoot, "composer.json"))
	}
	if err == nil {
		lock, err = readProjectFile(root, filepath.Join(request.ComposerRoot, "composer.lock"))
	}
	if err != nil {
		if request.Profile != nil && root != nil {
			_, manifestErr := root.Lstat(filepath.Join(request.ComposerRoot, "composer.json"))
			_, lockErr := root.Lstat(filepath.Join(request.ComposerRoot, "composer.lock"))
			if os.IsNotExist(manifestErr) && os.IsNotExist(lockErr) {
				result.Sources = append(result.Sources, Source{ID: "composer", Status: "not_applicable", Note: "No Composer manifest or lock; profile release status is assessed separately."})
				if !result.Complete {
					result.ExitCode = 3
				}
				return result
			}
		}
		result.Complete = false
		result.Sources = append(result.Sources, Source{ID: "composer", Status: "unavailable", Note: err.Error()})
		result.ExitCode = 3
		return result
	}
	packages, local, err := inventory(composer, lock)
	if err != nil {
		result.Complete = false
		result.ExitCode = 2
		result.Notes = append(result.Notes, err.Error())
		return result
	}
	identity := "platform-independent-v2/" + fmt.Sprintf("%x", sha256.Sum256(append(append([]byte{}, composer...), lock...))) + "/" + request.Toolset
	validate := func(data []byte) error {
		copy, loc, err := inventory(composer, lock)
		if err != nil {
			return err
		}
		_, err = parseComposer(data, copy, loc)
		return err
	}
	var fetch func(context.Context) ([]byte, error)
	if request.Composer != nil {
		fetch = func(ctx context.Context) ([]byte, error) { return request.Composer(ctx, composer, lock) }
	}
	data, source, err := cached(ctx, "composer", identity, request.Settings, request.Now, validate, fetch)
	if err != nil {
		source.Note = err.Error()
		result.Complete = false
	} else {
		diagnostics, parseErr := parseComposer(data, packages, local)
		if parseErr != nil {
			source.Note = parseErr.Error()
			result.Complete = false
		} else if diagnostics {
			source.Note = "Composer emitted diagnostics; repository coverage may be incomplete."
			result.Complete = false
		}
		if source.Status == "stale" {
			result.Complete = false
		}
	}
	result.Sources = append(result.Sources, source)
	localCount := 0
	for name, item := range packages {
		if local[name] {
			localCount++
			item.Compatibility = "local_path_package"
		}
		if request.Options.Direct && !item.Direct || request.Options.NoDev && item.Dev {
			continue
		}
		if request.Options.Security && len(item.Advisories) == 0 {
			continue
		}
		if item.Latest == "" && !item.Abandoned && len(item.Advisories) == 0 {
			continue
		}
		if item.UpdateType == "none" && !item.Abandoned && len(item.Advisories) == 0 {
			continue
		}
		result.Packages = append(result.Packages, item)
	}
	if localCount > 0 {
		result.Complete = false
		result.Notes = append(result.Notes, fmt.Sprintf("%d local path package(s) have no remote release assessment; their locked versions remain included in the advisory query.", localCount))
	}
	slices.SortFunc(result.Packages, func(a, b Package) int {
		if len(a.Advisories) > 0 && len(b.Advisories) == 0 {
			return -1
		}
		if len(b.Advisories) > 0 && len(a.Advisories) == 0 {
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	currentComposer, readErr := readProjectFile(root, filepath.Join(request.ComposerRoot, "composer.json"))
	currentLock, lockErr := readProjectFile(root, filepath.Join(request.ComposerRoot, "composer.lock"))
	if readErr != nil || lockErr != nil || string(currentComposer) != string(composer) || string(currentLock) != string(lock) {
		result.Complete = false
		result.Notes = append(result.Notes, "Composer inputs changed while checking updates; rerun the command.")
	}
	if !result.Complete {
		result.ExitCode = 3
	}
	return result
}
