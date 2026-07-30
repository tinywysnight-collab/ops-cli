package config

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tinywysnight-collab/ops-cli/internal/fsutil"
	"github.com/tinywysnight-collab/ops-cli/internal/lock"
)

// Store applies loss-minimizing, transactional mutations to config.yaml.
type Store struct {
	path     string
	lockPath string
}

// NewStore returns a configuration store serialized by lockPath.
func NewStore(path, lockPath string) *Store {
	return &Store{path: path, lockPath: lockPath}
}

// AddAccount appends a new account without overwriting an existing alias.
func (s *Store) AddAccount(ctx context.Context, alias string, account Account) error {
	return s.update(ctx, func(doc *yaml.Node, _ *Config) error {
		return addAccountNode(doc, alias, account)
	})
}

// AddCluster appends a new cluster without overwriting an existing alias.
func (s *Store) AddCluster(ctx context.Context, alias string, cluster Cluster) error {
	return s.update(ctx, func(doc *yaml.Node, _ *Config) error {
		return addClusterNode(doc, alias, cluster)
	})
}

// DeleteCluster removes only the named cluster.
func (s *Store) DeleteCluster(ctx context.Context, alias string) error {
	return s.update(ctx, func(doc *yaml.Node, _ *Config) error {
		return deleteMappingEntry(doc, "clusters", alias)
	})
}

// DeleteAccount removes an unreferenced account. References are checked under
// the write lock against the latest configuration.
func (s *Store) DeleteAccount(ctx context.Context, alias string) error {
	return s.update(ctx, func(doc *yaml.Node, cfg *Config) error {
		var refs []string
		for clusterAlias, cluster := range cfg.Clusters {
			if cluster.Account == alias {
				refs = append(refs, clusterAlias)
			}
		}
		if len(refs) > 0 {
			sort.Strings(refs)
			return fmt.Errorf("cannot delete account %q; referenced by clusters: %s; delete these clusters first with `opsx cluster delete`", alias, strings.Join(refs, ", "))
		}
		return deleteMappingEntry(doc, "accounts", alias)
	})
}

func (s *Store) update(ctx context.Context, mutate func(*yaml.Node, *Config) error) error {
	return lock.With(ctx, s.lockPath, func() error {
		data, err := os.ReadFile(s.path)
		if err != nil {
			return fmt.Errorf("read config %s: %w", s.path, err)
		}
		cfg, err := decodeAndValidate(data)
		if err != nil {
			return err
		}
		doc, err := parseEditableYAML(data)
		if err != nil {
			return err
		}
		if err := mutate(doc, cfg); err != nil {
			return err
		}
		next, _, err := encodeAndValidate(doc)
		if err != nil {
			return err
		}
		info, err := os.Stat(s.path)
		if err != nil {
			return fmt.Errorf("stat config %s: %w", s.path, err)
		}
		return fsutil.AtomicWrite(s.path, next, info.Mode().Perm())
	})
}

func parseEditableYAML(data []byte) (*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse editable config: %w", err)
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("parse additional YAML document: %w", err)
		}
		return nil, fmt.Errorf("multiple YAML documents are not supported for interactive mutation")
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("config root must be a YAML mapping")
	}
	if err := rejectAdvancedYAML(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func rejectAdvancedYAML(node *yaml.Node) error {
	if node.Anchor != "" {
		return fmt.Errorf("YAML anchors are not supported for interactive mutation")
	}
	if node.Kind == yaml.AliasNode {
		return fmt.Errorf("YAML aliases are not supported for interactive mutation")
	}
	if node.Tag == "!!merge" || node.Value == "<<" {
		return fmt.Errorf("YAML merge keys are not supported for interactive mutation")
	}
	for _, child := range node.Content {
		if err := rejectAdvancedYAML(child); err != nil {
			return err
		}
	}
	return nil
}

func mappingSection(doc *yaml.Node, section string) (*yaml.Node, error) {
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != section {
			continue
		}
		value := root.Content[i+1]
		if value.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("%s must be a YAML mapping", section)
		}
		return value, nil
	}
	return nil, fmt.Errorf("config is missing %s mapping", section)
}

func addAccountNode(doc *yaml.Node, alias string, account Account) error {
	return addMappingEntry(doc, "accounts", alias, account)
}

func addClusterNode(doc *yaml.Node, alias string, cluster Cluster) error {
	return addMappingEntry(doc, "clusters", alias, cluster)
}

func addMappingEntry(doc *yaml.Node, section, alias string, value any) error {
	mapping, err := mappingSection(doc, section)
	if err != nil {
		return err
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == alias {
			return fmt.Errorf("%s alias %q already exists", section, alias)
		}
	}
	var valueNode yaml.Node
	if err := valueNode.Encode(value); err != nil {
		return fmt.Errorf("encode %s %q: %w", section, alias, err)
	}
	mapping.Style = 0
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: alias},
		&valueNode,
	)
	return nil
}

func deleteMappingEntry(doc *yaml.Node, section, alias string) error {
	mapping, err := mappingSection(doc, section)
	if err != nil {
		return err
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != alias {
			continue
		}
		mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
		if len(mapping.Content) == 0 {
			mapping.Style = yaml.FlowStyle
		}
		return nil
	}
	return fmt.Errorf("%s alias %q does not exist", section, alias)
}

func encodeAndValidate(doc *yaml.Node) ([]byte, *Config, error) {
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return nil, nil, fmt.Errorf("encode config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, nil, fmt.Errorf("close config encoder: %w", err)
	}

	cfg, err := decodeAndValidate(out.Bytes())
	if err != nil {
		return nil, nil, err
	}
	return out.Bytes(), cfg, nil
}

func decodeAndValidate(data []byte) (*Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse candidate config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}
