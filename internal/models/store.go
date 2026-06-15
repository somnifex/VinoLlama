package models

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	FormatGGUF      = "gguf"
	SourceCopy      = "copy"
	SourceLink      = "link"
	SourceReference = "reference"
)

type Manifest struct {
	Name         string
	Format       string
	Path         string
	Size         int64
	Quantization string
	Parameters   string
	BackendHint  string
	CreatedAt    time.Time
	ModifiedAt   time.Time
	Template     string
	Source       string
}

type Store struct {
	Dir          string
	ManifestsDir string
	BlobsDir     string
}

type ImportRequest struct {
	Name string
	Path string
	Mode string
}

type DeleteResult struct {
	ManifestPath string
	ModelPath    string
	FileDeleted  bool
}

func NewStore(dir string) (Store, error) {
	if dir == "" {
		return Store{}, errors.New("model directory must not be empty")
	}
	return Store{
		Dir:          dir,
		ManifestsDir: filepath.Join(dir, "manifests"),
		BlobsDir:     filepath.Join(dir, "blobs"),
	}, nil
}

func (s Store) Init() error {
	for _, dir := range []string{s.Dir, s.ManifestsDir, s.BlobsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return nil
}

func (s Store) Import(req ImportRequest) (Manifest, error) {
	if err := s.Init(); err != nil {
		return Manifest{}, err
	}
	name, err := CleanName(req.Name)
	if err != nil {
		return Manifest{}, err
	}
	mode := req.Mode
	if mode == "" {
		mode = SourceReference
	}
	if !ValidImportMode(mode) {
		return Manifest{}, fmt.Errorf("import mode must be one of reference, copy, link, got %q", mode)
	}
	if _, err := os.Stat(s.ManifestPath(name)); err == nil {
		return Manifest{}, fmt.Errorf("model %q already exists; remove it first with `vinollama rm %s`", name, name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Manifest{}, fmt.Errorf("inspect existing manifest for %s: %w", name, err)
	}
	if strings.ToLower(filepath.Ext(req.Path)) != ".gguf" {
		return Manifest{}, fmt.Errorf("model path must point to a .gguf file, got %q", req.Path)
	}
	absPath, err := filepath.Abs(req.Path)
	if err != nil {
		return Manifest{}, fmt.Errorf("resolve model path: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return Manifest{}, fmt.Errorf("inspect model file %s: %w", absPath, err)
	}
	if info.IsDir() {
		return Manifest{}, fmt.Errorf("%s is a directory, not a GGUF file", absPath)
	}

	storedPath := absPath
	source := mode
	switch mode {
	case SourceCopy:
		storedPath, err = s.copyBlob(name, absPath)
		if err != nil {
			return Manifest{}, err
		}
	case SourceLink:
		storedPath, err = s.linkBlob(name, absPath)
		if err != nil {
			storedPath = absPath
			source = SourceReference
		}
	}

	now := time.Now().UTC()
	metadata := InferMetadata(filepath.Base(absPath))
	manifest := Manifest{
		Name:         name,
		Format:       FormatGGUF,
		Path:         storedPath,
		Size:         info.Size(),
		Quantization: metadata.Quantization,
		Parameters:   metadata.Parameters,
		BackendHint:  "auto",
		CreatedAt:    now,
		ModifiedAt:   now,
		Template:     "auto",
		Source:       source,
	}
	if err := s.WriteManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (s Store) List() ([]Manifest, error) {
	if err := s.Init(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.ManifestsDir)
	if err != nil {
		return nil, fmt.Errorf("read manifests directory: %w", err)
	}
	var manifests []Manifest
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".yaml" {
			continue
		}
		manifest, err := s.ReadManifest(strings.TrimSuffix(entry.Name(), ".yaml"))
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, manifest)
	}
	sort.Slice(manifests, func(i, j int) bool {
		return strings.ToLower(manifests[i].Name) < strings.ToLower(manifests[j].Name)
	})
	return manifests, nil
}

func (s Store) Delete(name string, deleteFile bool) (DeleteResult, error) {
	clean, err := CleanName(name)
	if err != nil {
		return DeleteResult{}, err
	}
	manifest, err := s.ReadManifest(clean)
	if err != nil {
		return DeleteResult{}, err
	}
	result := DeleteResult{
		ManifestPath: s.ManifestPath(clean),
		ModelPath:    manifest.Path,
	}
	if deleteFile {
		if err := removeModelFile(manifest.Path); err != nil {
			return DeleteResult{}, err
		}
		result.FileDeleted = true
	}
	if err := os.Remove(result.ManifestPath); err != nil {
		return DeleteResult{}, fmt.Errorf("remove manifest %s: %w", result.ManifestPath, err)
	}
	return result, nil
}

func (s Store) ReadManifest(name string) (Manifest, error) {
	clean, err := CleanName(name)
	if err != nil {
		return Manifest{}, err
	}
	data, err := os.ReadFile(s.ManifestPath(clean))
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest for %s: %w", clean, err)
	}
	return ParseManifest(data)
}

func (s Store) WriteManifest(manifest Manifest) error {
	clean, err := CleanName(manifest.Name)
	if err != nil {
		return err
	}
	manifest.Name = clean
	if err := s.Init(); err != nil {
		return err
	}
	data := FormatManifest(manifest)
	if err := os.WriteFile(s.ManifestPath(clean), data, 0o644); err != nil {
		return fmt.Errorf("write manifest for %s: %w", clean, err)
	}
	return nil
}

func (s Store) ManifestPath(name string) string {
	return filepath.Join(s.ManifestsDir, name+".yaml")
}

func (s Store) copyBlob(name, source string) (string, error) {
	dest := filepath.Join(s.BlobsDir, name+".gguf")
	if _, err := os.Stat(dest); err == nil {
		return "", fmt.Errorf("managed model file already exists at %s", dest)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect destination %s: %w", dest, err)
	}
	in, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("open source model %s: %w", source, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", fmt.Errorf("create managed model copy %s: %w", dest, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return "", fmt.Errorf("copy model to %s: %w", dest, err)
	}
	return dest, nil
}

func (s Store) linkBlob(name, source string) (string, error) {
	dest := filepath.Join(s.BlobsDir, name+".gguf")
	if _, err := os.Stat(dest); err == nil {
		return "", fmt.Errorf("managed model link already exists at %s", dest)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect destination %s: %w", dest, err)
	}
	if err := os.Symlink(source, dest); err != nil {
		return "", fmt.Errorf("create symlink %s -> %s: %w", dest, source, err)
	}
	return dest, nil
}

func removeModelFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect model file %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory; refusing to delete it as a model file", path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete model file %s: %w", path, err)
	}
	return nil
}

func ValidImportMode(mode string) bool {
	switch mode {
	case SourceReference, SourceCopy, SourceLink:
		return true
	default:
		return false
	}
}

func CleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("model name must not be empty")
	}
	if name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("model name %q must not contain path separators", name)
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return "", fmt.Errorf("model name %q contains unsupported character %q", name, r)
	}
	return name, nil
}

type Metadata struct {
	Parameters   string
	Quantization string
}

func InferMetadata(filename string) Metadata {
	lower := strings.ToLower(strings.TrimSuffix(filename, filepath.Ext(filename)))
	metadata := Metadata{
		Parameters:   "unknown",
		Quantization: "unknown",
	}

	paramRe := regexp.MustCompile(`(?:^|[-_])(\d+(?:\.\d+)?)([bm])(?:$|[-_.])`)
	if match := paramRe.FindStringSubmatch(lower); len(match) == 3 {
		metadata.Parameters = strings.ToUpper(match[1] + match[2])
	}

	quantRe := regexp.MustCompile(`(?:^|[-_])(q[0-9]+(?:_[a-z0-9]+)*)(?:$|[-_.])`)
	if match := quantRe.FindStringSubmatch(lower); len(match) == 2 {
		metadata.Quantization = strings.ToUpper(match[1])
	}
	return metadata
}

func FormatManifest(manifest Manifest) []byte {
	var b strings.Builder
	writeString := func(key, value string) {
		fmt.Fprintf(&b, "%s: %s\n", key, strconv.Quote(value))
	}
	writeString("name", manifest.Name)
	writeString("format", manifest.Format)
	writeString("path", manifest.Path)
	fmt.Fprintf(&b, "size: %d\n", manifest.Size)
	writeString("quantization", valueOrUnknown(manifest.Quantization))
	writeString("parameters", valueOrUnknown(manifest.Parameters))
	writeString("backend_hint", valueOrDefault(manifest.BackendHint, "auto"))
	writeString("created_at", manifest.CreatedAt.UTC().Format(time.RFC3339))
	writeString("modified_at", manifest.ModifiedAt.UTC().Format(time.RFC3339))
	writeString("template", valueOrDefault(manifest.Template, "auto"))
	writeString("source", valueOrDefault(manifest.Source, SourceReference))
	return []byte(b.String())
}

func ParseManifest(data []byte) (Manifest, error) {
	values := map[string]string{}
	for lineNo, raw := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			return Manifest{}, fmt.Errorf("invalid manifest line %d: %q", lineNo+1, raw)
		}
		key := strings.TrimSpace(parts[0])
		value, err := parseScalar(parts[1])
		if err != nil {
			return Manifest{}, fmt.Errorf("invalid manifest line %d: %w", lineNo+1, err)
		}
		values[key] = value
	}

	size, err := strconv.ParseInt(values["size"], 10, 64)
	if err != nil {
		return Manifest{}, fmt.Errorf("manifest size must be an integer: %w", err)
	}
	createdAt, err := parseTime(values["created_at"])
	if err != nil {
		return Manifest{}, fmt.Errorf("manifest created_at is invalid: %w", err)
	}
	modifiedAt, err := parseTime(values["modified_at"])
	if err != nil {
		return Manifest{}, fmt.Errorf("manifest modified_at is invalid: %w", err)
	}

	manifest := Manifest{
		Name:         values["name"],
		Format:       valueOrDefault(values["format"], FormatGGUF),
		Path:         values["path"],
		Size:         size,
		Quantization: valueOrUnknown(values["quantization"]),
		Parameters:   valueOrUnknown(values["parameters"]),
		BackendHint:  valueOrDefault(values["backend_hint"], "auto"),
		CreatedAt:    createdAt,
		ModifiedAt:   modifiedAt,
		Template:     valueOrDefault(values["template"], "auto"),
		Source:       valueOrDefault(values["source"], SourceReference),
	}
	if _, err := CleanName(manifest.Name); err != nil {
		return Manifest{}, err
	}
	if manifest.Path == "" {
		return Manifest{}, errors.New("manifest path must not be empty")
	}
	return manifest, nil
}

func parseScalar(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, `"`) {
		unquoted, err := strconv.Unquote(value)
		if err != nil {
			return "", err
		}
		return unquoted, nil
	}
	return strings.Trim(value, `'`), nil
}

func parseTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, value)
}

func valueOrUnknown(value string) string {
	return valueOrDefault(value, "unknown")
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
