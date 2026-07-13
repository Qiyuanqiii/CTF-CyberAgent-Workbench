package skills

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"cyberagent-workbench/internal/domain"
)

type registryEntry struct {
	manifest Manifest
	content  []byte
}

// Registry is immutable after construction. It intentionally exposes no Skill
// content or mutation API in the first skill.v1 slice.
type Registry struct {
	entries map[string]registryEntry
}

func LoadFS(source fs.FS, root string) (*Registry, error) {
	if source == nil {
		return nil, errors.New("skill filesystem is required")
	}
	if root == "" || !fs.ValidPath(root) {
		return nil, errors.New("skill registry root must be a valid relative path")
	}
	if err := validateDirectoryPath(source, root); err != nil {
		return nil, err
	}
	directories, err := fs.ReadDir(source, root)
	if err != nil {
		return nil, fmt.Errorf("read skill registry: %w", err)
	}
	if len(directories) == 0 || len(directories) > MaxSkills {
		return nil, fmt.Errorf("skill registry must contain between 1 and %d entries", MaxSkills)
	}
	registry := &Registry{entries: make(map[string]registryEntry, len(directories))}
	for _, directory := range directories {
		info, infoErr := directory.Info()
		if infoErr != nil || info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() || !validName(directory.Name()) {
			return nil, fmt.Errorf("invalid skill registry entry %q", directory.Name())
		}
		base := path.Join(root, directory.Name())
		if err := validateRegularFile(source, base, "manifest.json"); err != nil {
			return nil, err
		}
		rawManifest, err := readBoundedFile(source, path.Join(base, "manifest.json"), MaxManifestBytes)
		if err != nil {
			return nil, err
		}
		manifest, err := decodeManifest(rawManifest)
		if err != nil {
			return nil, fmt.Errorf("skill %q manifest: %w", directory.Name(), err)
		}
		if manifest.Name != directory.Name() {
			return nil, fmt.Errorf("skill directory %q does not match manifest name %q", directory.Name(), manifest.Name)
		}
		if err := validateContentPath(manifest.ContentPath); err != nil {
			return nil, fmt.Errorf("skill %q manifest: %w", manifest.Name, err)
		}
		if err := validateRegularFile(source, base, manifest.ContentPath); err != nil {
			return nil, err
		}
		content, err := readBoundedFile(source, path.Join(base, manifest.ContentPath), MaxContentBytes)
		if err != nil {
			return nil, err
		}
		if err := manifest.Validate(content); err != nil {
			return nil, fmt.Errorf("skill %q manifest: %w", manifest.Name, err)
		}
		if _, exists := registry.entries[manifest.Name]; exists {
			return nil, fmt.Errorf("duplicate skill %q", manifest.Name)
		}
		registry.entries[manifest.Name] = registryEntry{
			manifest: cloneManifest(manifest),
			content:  bytes.Clone(content),
		}
	}
	return registry, nil
}

func (r *Registry) List(profile domain.Profile) []Manifest {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.entries))
	for name, entry := range r.entries {
		if profile != "" && !containsProfile(entry.manifest.Profiles, profile) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	manifests := make([]Manifest, 0, len(names))
	for _, name := range names {
		manifests = append(manifests, cloneManifest(r.entries[name].manifest))
	}
	return manifests
}

func (r *Registry) Get(name string) (Manifest, bool) {
	if r == nil || !validName(name) {
		return Manifest{}, false
	}
	entry, ok := r.entries[name]
	if !ok {
		return Manifest{}, false
	}
	return cloneManifest(entry.manifest), true
}

func (r *Registry) Validate() error {
	if r == nil || len(r.entries) == 0 || len(r.entries) > MaxSkills {
		return errors.New("skill registry is empty or exceeds its bound")
	}
	names := make([]string, 0, len(r.entries))
	for name := range r.entries {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry := r.entries[name]
		if name != entry.manifest.Name {
			return fmt.Errorf("skill registry key %q does not match manifest name", name)
		}
		if err := entry.manifest.Validate(entry.content); err != nil {
			return fmt.Errorf("skill %q manifest: %w", name, err)
		}
	}
	return nil
}

func decodeManifest(raw []byte) (Manifest, error) {
	if len(raw) == 0 || len(raw) > MaxManifestBytes {
		return Manifest{}, fmt.Errorf("manifest must contain between 1 and %d bytes", MaxManifestBytes)
	}
	if !utf8.Valid(raw) {
		return Manifest{}, errors.New("manifest must be valid UTF-8 JSON")
	}
	if err := rejectDuplicateManifestFields(raw); err != nil {
		return Manifest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode strict JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Manifest{}, errors.New("manifest contains trailing JSON data")
	}
	return manifest, nil
}

func rejectDuplicateManifestFields(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode strict JSON: %w", err)
	}
	if opening != json.Delim('{') {
		return errors.New("manifest must be a JSON object")
	}
	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("decode strict JSON: %w", err)
		}
		field, ok := token.(string)
		if !ok {
			return errors.New("manifest contains a non-string field name")
		}
		if _, exists := seen[field]; exists {
			return fmt.Errorf("manifest contains duplicate field %q", field)
		}
		seen[field] = struct{}{}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return fmt.Errorf("decode strict JSON field %q: %w", field, err)
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return errors.New("manifest JSON object is not closed")
	}
	return nil
}

func readBoundedFile(source fs.FS, name string, limit int) ([]byte, error) {
	file, err := source.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open skill file %q: %w", name, err)
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, fmt.Errorf("read skill file %q: %w", name, err)
	}
	if len(content) > limit {
		return nil, fmt.Errorf("skill file %q exceeds %d bytes", name, limit)
	}
	return content, nil
}

func validateRegularFile(source fs.FS, base string, relative string) error {
	current := base
	parts := strings.Split(relative, "/")
	for index, part := range parts {
		entries, err := fs.ReadDir(source, current)
		if err != nil {
			return fmt.Errorf("read skill path %q: %w", current, err)
		}
		var found fs.DirEntry
		for _, entry := range entries {
			if entry.Name() == part {
				found = entry
				break
			}
		}
		if found == nil {
			return fmt.Errorf("skill file %q not found", path.Join(current, part))
		}
		info, err := found.Info()
		if err != nil {
			return fmt.Errorf("inspect skill path %q: %w", path.Join(current, part), err)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("skill path %q cannot contain symbolic links", path.Join(current, part))
		}
		last := index == len(parts)-1
		if !last && !info.IsDir() {
			return fmt.Errorf("skill path %q must be a directory", path.Join(current, part))
		}
		if last {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("skill path %q must be a regular file", path.Join(current, part))
			}
		}
		current = path.Join(current, part)
	}
	return nil
}

func validateDirectoryPath(source fs.FS, relative string) error {
	if relative == "." {
		return nil
	}
	current := "."
	for _, part := range strings.Split(relative, "/") {
		entries, err := fs.ReadDir(source, current)
		if err != nil {
			return fmt.Errorf("read skill registry path %q: %w", current, err)
		}
		var found fs.DirEntry
		for _, entry := range entries {
			if entry.Name() == part {
				found = entry
				break
			}
		}
		if found == nil {
			return fmt.Errorf("skill registry path %q not found", path.Join(current, part))
		}
		info, err := found.Info()
		if err != nil {
			return fmt.Errorf("inspect skill registry path %q: %w", path.Join(current, part), err)
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("skill registry path %q must be a real directory", path.Join(current, part))
		}
		current = path.Join(current, part)
	}
	return nil
}

func containsProfile(profiles []domain.Profile, profile domain.Profile) bool {
	for _, candidate := range profiles {
		if candidate == profile {
			return true
		}
	}
	return false
}
