// Package mihomo 提供 Mihomo 配置目录解析与 Provider 安全写入。
package mihomo

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const configFilename = "config.yaml"

// Provider 描述可对外展示的 Provider 元数据。
type Provider struct {
	Name   string `json:"name"`
	HasURL bool   `json:"has_url"`
}

// Settings 描述 Mihomo 配置目录与 Provider 列表。
type Settings struct {
	ConfigDir        string     `json:"config_dir"`
	DefaultConfigDir string     `json:"default_config_dir"`
	ConfigPath       string     `json:"config_path"`
	Providers        []Provider `json:"providers"`
}

// AddProviderRequest 描述新增 Provider 所需的最小输入。
type AddProviderRequest struct {
	ConfigDir string `json:"config_dir"`
	Name      string `json:"name"`
	URL       string `json:"url"`
}

// DefaultConfigDir 返回 Mihomo 的默认配置目录。
func DefaultConfigDir() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("NEXUSBRIDGE_MIHOMO_CONFIG_DIR")); configured != "" {
		return resolveConfigDir(configured)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve mihomo home directory: %w", err)
	}
	homeDefault := filepath.Join(homeDir, ".config", "mihomo")
	if _, err := os.Stat(homeDefault); err == nil || !errors.Is(err, os.ErrNotExist) {
		return filepath.Clean(homeDefault), nil
	}
	if xdgConfigHome := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdgConfigHome != "" {
		return filepath.Join(xdgConfigHome, "mihomo"), nil
	}
	return filepath.Clean(homeDefault), nil
}

// Load 读取指定目录中的 Provider 元数据。
func Load(configDir string) (Settings, error) {
	resolvedDir, defaultDir, err := resolveRequestedConfigDir(configDir)
	if err != nil {
		return Settings{}, err
	}
	document, configPath, err := readDocument(resolvedDir)
	if err != nil {
		return Settings{}, err
	}
	providersNode, err := mappingValue(document.Content[0], "proxy-providers")
	if err != nil {
		return Settings{}, err
	}
	providers, err := listProviders(providersNode)
	if err != nil {
		return Settings{}, err
	}
	return Settings{
		ConfigDir:        resolvedDir,
		DefaultConfigDir: defaultDir,
		ConfigPath:       configPath,
		Providers:        providers,
	}, nil
}

// AddProvider 在不覆盖同名项的前提下追加 HTTP Provider。
func AddProvider(request AddProviderRequest) (Settings, error) {
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return Settings{}, errors.New("mihomo provider name is required")
	}
	providerURL := strings.TrimSpace(request.URL)
	parsedURL, err := url.ParseRequestURI(providerURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return Settings{}, errors.New("mihomo provider url must be a valid HTTP or HTTPS URL")
	}
	resolvedDir, _, err := resolveRequestedConfigDir(request.ConfigDir)
	if err != nil {
		return Settings{}, err
	}
	document, configPath, err := readDocument(resolvedDir)
	if err != nil {
		return Settings{}, err
	}
	root := document.Content[0]
	providersNode, err := mappingValue(root, "proxy-providers")
	if err != nil {
		return Settings{}, err
	}
	if providersNode == nil {
		providersNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "proxy-providers"},
			providersNode,
		)
	}
	for index := 0; index < len(providersNode.Content); index += 2 {
		if providersNode.Content[index].Value == name {
			return Settings{}, fmt.Errorf("mihomo provider %q already exists", name)
		}
	}
	providersNode.Content = append(providersNode.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name},
		newProviderNode(name, providerURL),
	)
	if err := writeDocument(configPath, document); err != nil {
		return Settings{}, err
	}
	return Load(resolvedDir)
}

func resolveRequestedConfigDir(configDir string) (string, string, error) {
	defaultDir, err := DefaultConfigDir()
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(configDir) == "" {
		return defaultDir, defaultDir, nil
	}
	resolvedDir, err := resolveConfigDir(configDir)
	if err != nil {
		return "", "", err
	}
	return resolvedDir, defaultDir, nil
}

func resolveConfigDir(configDir string) (string, error) {
	value := strings.TrimSpace(configDir)
	if value == "" {
		return "", errors.New("mihomo config directory is required")
	}
	if value == "~" || strings.HasPrefix(value, "~/") || strings.HasPrefix(value, `~\`) {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve mihomo home directory: %w", err)
		}
		if value == "~" {
			value = homeDir
		} else {
			value = filepath.Join(homeDir, value[2:])
		}
	}
	resolved, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve mihomo config directory: %w", err)
	}
	return filepath.Clean(resolved), nil
}

func readDocument(configDir string) (*yaml.Node, string, error) {
	configPath := filepath.Join(configDir, configFilename)
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, configPath, fmt.Errorf("read mihomo config %s: %w", configPath, err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, configPath, fmt.Errorf("parse mihomo config %s: %w", configPath, err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, configPath, fmt.Errorf("mihomo config %s must contain a YAML mapping", configPath)
	}
	return &document, configPath, nil
}

func mappingValue(mapping *yaml.Node, key string) (*yaml.Node, error) {
	for index := 0; index < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value != key {
			continue
		}
		value := mapping.Content[index+1]
		if value.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("mihomo %s must be a YAML mapping", key)
		}
		return value, nil
	}
	return nil, nil
}

func listProviders(providersNode *yaml.Node) ([]Provider, error) {
	if providersNode == nil {
		return []Provider{}, nil
	}
	providers := make([]Provider, 0, len(providersNode.Content)/2)
	for index := 0; index < len(providersNode.Content); index += 2 {
		name := strings.TrimSpace(providersNode.Content[index].Value)
		providerNode := providersNode.Content[index+1]
		if providerNode.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("mihomo provider %q must be a YAML mapping", name)
		}
		hasURL := false
		for fieldIndex := 0; fieldIndex < len(providerNode.Content); fieldIndex += 2 {
			if providerNode.Content[fieldIndex].Value == "url" {
				hasURL = strings.TrimSpace(providerNode.Content[fieldIndex+1].Value) != ""
				break
			}
		}
		providers = append(providers, Provider{Name: name, HasURL: hasURL})
	}
	return providers, nil
}

func newProviderNode(name, providerURL string) *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "type"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "http"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "url"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: providerURL},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "interval"},
		{Kind: yaml.ScalarNode, Tag: "!!int", Value: "86400"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "health-check"},
		{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "enable"},
			{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "url"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "https://www.gstatic.com/generate_204"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "interval"},
			{Kind: yaml.ScalarNode, Tag: "!!int", Value: "300"},
		}},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "override"},
		{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "additional-prefix"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "[" + name + "]"},
		}},
	}}
}

func writeDocument(configPath string, document *yaml.Node) error {
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return fmt.Errorf("encode mihomo config: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("finish mihomo config encoding: %w", err)
	}
	info, err := os.Stat(configPath)
	if err != nil {
		return fmt.Errorf("inspect mihomo config %s: %w", configPath, err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(configPath), ".config.yaml.*")
	if err != nil {
		return fmt.Errorf("create temporary mihomo config: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set temporary mihomo config permissions: %w", err)
	}
	if _, err := temporary.Write(output.Bytes()); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary mihomo config: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary mihomo config: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary mihomo config: %w", err)
	}
	if err := os.Rename(temporaryPath, configPath); err != nil {
		return fmt.Errorf("replace mihomo config %s: %w", configPath, err)
	}
	return nil
}
