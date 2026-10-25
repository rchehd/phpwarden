package projectupdates

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/rchehd/phpwarden/internal/cache"
	"github.com/rchehd/phpwarden/internal/config"
)

type entry struct {
	Version int       `json:"version"`
	Fetched time.Time `json:"fetched"`
	Data    []byte    `json:"data"`
}

func cached(ctx context.Context, id, identity string, settings config.Updates, now time.Time, validate func([]byte) error, fetch func(context.Context) ([]byte, error)) ([]byte, Source, error) {
	source := Source{ID: id, Status: "unavailable"}
	directory, err := cache.Directory("updates")
	var store *cache.Store
	if err == nil {
		store, err = cache.Open(directory)
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte("updates-v1/"+id+"/"+identity)))
	var old entry
	if store != nil {
		defer store.Close()
		release, err := store.Lock(ctx, key)
		if err != nil {
			return nil, source, err
		}
		defer release()
		if data, err := store.Read(key); err == nil && json.Unmarshal(data, &old) == nil && old.Version == 1 && !old.Fetched.IsZero() && !old.Fetched.After(now) && validate(old.Data) == nil {
			if now.Sub(old.Fetched) <= time.Duration(settings.MaxAgeHours)*time.Hour {
				source.Status, source.FetchedAt = "cache", old.Fetched.UTC().Format(time.RFC3339)
				return old.Data, source, nil
			}
		} else {
			old = entry{}
		}
	}
	var data []byte
	if settings.Offline {
		err = fmt.Errorf("offline mode: no fresh cached %s data", id)
	} else if fetch == nil {
		err = fmt.Errorf("%s source is unavailable", id)
	} else {
		data, err = fetch(ctx)
		if err == nil {
			err = validate(data)
		}
	}
	if err != nil {
		if !old.Fetched.IsZero() && now.Sub(old.Fetched) <= time.Duration(settings.MaxStaleHours)*time.Hour && validate(old.Data) == nil {
			source.Status, source.FetchedAt, source.Note = "stale", old.Fetched.UTC().Format(time.RFC3339), "Refresh unavailable; using explicitly dated stale data."
			return old.Data, source, nil
		}
		return nil, source, err
	}
	source.Status, source.FetchedAt = "network", now.UTC().Format(time.RFC3339)
	if store != nil {
		payload, _ := json.Marshal(entry{Version: 1, Fetched: now.UTC(), Data: data})
		if err := store.Write(key, payload); err != nil {
			source.Note = "Data was fetched but could not be cached."
		}
	} else {
		source.Note = "Data was fetched without a writable private cache."
	}
	return data, source, nil
}

func readProjectFile(root *os.Root, name string) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return nil, fmt.Errorf("%s must be a readable regular file at most 16 MiB", name)
	}
	return root.ReadFile(name)
}
