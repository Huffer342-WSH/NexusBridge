package organizer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"nexusbridge/internal/llm"
)

type Options struct {
	LibraryRoot string
	DryRun      bool
}

type Result struct {
	RelativeDir string
	Filename    string
	TargetPath  string
	LLMResponse string
	Confidence  float64
}

type Organizer struct {
	client *llm.Client
	opts   Options
}

func New(client *llm.Client, opts Options) *Organizer {
	return &Organizer{client: client, opts: opts}
}

func (o *Organizer) Organize(ctx context.Context, title, sourcePath string) (Result, error) {
	if strings.TrimSpace(o.opts.LibraryRoot) == "" {
		return Result{}, fmt.Errorf("media_library.root is required")
	}
	source, err := resolveSourceFile(sourcePath)
	if err != nil {
		return Result{}, err
	}
	suggestion, raw, err := o.client.SuggestOrganization(ctx, title, source)
	if err != nil {
		return Result{}, err
	}
	if err := validateRelativePath(suggestion.RelativeDir); err != nil {
		return Result{}, err
	}
	if err := validateFilename(suggestion.Filename); err != nil {
		return Result{}, err
	}
	target := filepath.Join(o.opts.LibraryRoot, filepath.FromSlash(suggestion.RelativeDir), suggestion.Filename)
	rootAbs, err := filepath.Abs(o.opts.LibraryRoot)
	if err != nil {
		return Result{}, err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return Result{}, err
	}
	if !strings.HasPrefix(strings.ToLower(targetAbs), strings.ToLower(rootAbs)+string(os.PathSeparator)) && strings.ToLower(targetAbs) != strings.ToLower(rootAbs) {
		return Result{}, fmt.Errorf("target path escapes media library root")
	}
	if !o.opts.DryRun {
		if err := os.MkdirAll(filepath.Dir(targetAbs), 0o755); err != nil {
			return Result{}, err
		}
		if err := os.Link(source, targetAbs); err != nil {
			return Result{}, fmt.Errorf("create hardlink: %w", err)
		}
	}
	return Result{
		RelativeDir: suggestion.RelativeDir,
		Filename:    suggestion.Filename,
		TargetPath:  targetAbs,
		LLMResponse: raw,
		Confidence:  suggestion.Confidence,
	}, nil
}

func resolveSourceFile(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("source path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return filepath.Abs(path)
	}
	var selected string
	err = filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || selected != "" || entry.IsDir() {
			return walkErr
		}
		selected = current
		return nil
	})
	if err != nil {
		return "", err
	}
	if selected == "" {
		return "", fmt.Errorf("directory contains no files")
	}
	return filepath.Abs(selected)
}

func validateRelativePath(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("relative_dir is required")
	}
	if filepath.IsAbs(value) || strings.Contains(value, "..") {
		return fmt.Errorf("relative_dir must be a safe relative path")
	}
	return nil
}

func validateFilename(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("filename is required")
	}
	if strings.ContainsAny(value, `<>:"/\|?*`) {
		return fmt.Errorf("filename contains invalid characters")
	}
	if regexp.MustCompile(`(?i)^(con|prn|aux|nul|com[1-9]|lpt[1-9])(\..*)?$`).MatchString(value) {
		return fmt.Errorf("filename uses reserved Windows name")
	}
	return nil
}
