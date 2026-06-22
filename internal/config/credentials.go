// credentials.go loads and validates the Enphase Cloud API credentials.
//
// The credentials live in a separate credentials.yaml file rather than
// config.yaml so that the non-secret settings in config.yaml can be shared or
// committed while the secrets stay local. credentials.yaml holds a list of one
// or more credential sets under a credentials: key; the app rotates across the
// pool to spread the per-key rate limit (10 req/min, 1000/month) and to fail
// over when a key is throttled. See config.go for the overall structure and
// internal/credentials for the pool that consumes these.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"enphase-monitor/internal/constants"

	"gopkg.in/yaml.v3"
)

// credentialsFile mirrors the on-disk layout of credentials.yaml: a list of
// credential sets, each reusing the shared APIConfig type.
type credentialsFile struct {
	Credentials []*APIConfig `yaml:"credentials"`
}

// LoadCredentials reads and parses the credentials file, returning the list of
// credential sets. A missing file is reported as a wrapped os.ErrNotExist so the
// caller can distinguish "no credentials file" from a genuine parse error. The
// returned credentials are validated by Config.ApplyCredentials, not here.
func LoadCredentials(filename string) ([]*APIConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}

	var creds credentialsFile
	if err := yaml.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credentials file: %w", err)
	}

	return creds.Credentials, nil
}

// ApplyCredentials attaches the credential pool to the config and validates it.
//
// Every credential set must carry a unique name plus the required secret fields
// (key, client_id, client_secret). The non-secret authorization_url and
// redirect_uri are filled from the shared api: block in config.yaml (c.API) when
// a credential does not set its own; authorization_url then falls back to the
// built-in Enphase token endpoint. The refresh_token of each set is trimmed of
// stray whitespace (a common copy/paste issue).
func (c *Config) ApplyCredentials(creds []*APIConfig) error {
	if len(creds) == 0 {
		return errors.New("no credentials configured: add a credentials: list to credentials.yaml (see credentials.yaml.example)")
	}

	seen := make(map[string]bool, len(creds))
	for i, cred := range creds {
		if cred == nil {
			return fmt.Errorf("credential %d is empty", i)
		}
		if cred.Name == "" {
			return fmt.Errorf("credential %d is missing a name (each entry under credentials: needs a unique name)", i)
		}
		if seen[cred.Name] {
			return fmt.Errorf("duplicate credential name %q (each entry under credentials: needs a unique name)", cred.Name)
		}
		seen[cred.Name] = true

		if cred.Key == "" || cred.ClientID == "" || cred.ClientSecret == "" {
			return fmt.Errorf("credential %q: key, client_id, and client_secret are required in credentials.yaml", cred.Name)
		}

		c.applySharedOAuth(cred)
		cred.RefreshToken = strings.TrimSpace(cred.RefreshToken)
	}

	c.Credentials = creds
	return nil
}

// applySharedOAuth fills a credential's non-secret OAuth settings from the shared
// api: block (when the credential omits its own), then defaults authorization_url
// to the built-in Enphase token endpoint. Per-credential values take precedence.
func (c *Config) applySharedOAuth(cred *APIConfig) {
	if cred.AuthorizationURL == "" && c.API != nil {
		cred.AuthorizationURL = c.API.AuthorizationURL
	}
	if cred.AuthorizationURL == "" {
		cred.AuthorizationURL = constants.EnphaseOAuthTokenURL
	}
	if cred.RedirectURI == "" && c.API != nil {
		cred.RedirectURI = c.API.RedirectURI
	}
}

// UpdateRefreshToken writes refreshToken into the named credential's entry in the
// credentials.yaml file at filename, preserving the rest of the file (comments,
// other entries, formatting). The edit is done on the YAML node tree so existing
// comments survive, then written atomically via a temp file + rename. It returns
// an error if the file cannot be parsed or no credential with the given name
// exists.
func UpdateRefreshToken(filename, name, refreshToken string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read credentials file: %w", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("failed to parse credentials file: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return errors.New("credentials file is empty or malformed")
	}

	credsSeq := mappingValue(doc.Content[0], "credentials")
	if credsSeq == nil || credsSeq.Kind != yaml.SequenceNode {
		return errors.New("credentials file has no credentials: list")
	}

	for _, entry := range credsSeq.Content {
		if entry.Kind != yaml.MappingNode {
			continue
		}
		if nameNode := mappingValue(entry, "name"); nameNode != nil && nameNode.Value == name {
			setMappingString(entry, "refresh_token", refreshToken)
			return writeFilePreservingMode(filename, &doc)
		}
	}

	return fmt.Errorf("no credential named %q in %s", name, filename)
}

// SeedCredential carries the identity fields of one Enphase application as read
// from the developer portal: the values needed to seed a credentials.yaml entry
// before its refresh token is obtained via --update-refresh-tokens.
type SeedCredential struct {
	Name         string
	Key          string
	ClientID     string
	ClientSecret string
}

// MergeSeedCredentials merges seeded portal credentials into the credentials.yaml
// file at filename, preserving comments, formatting, ordering, and — crucially —
// any existing refresh_token. For each seed whose name already exists, key,
// client_id, and client_secret are updated in place (non-empty values only) while
// refresh_token and any other fields are left untouched (a resync). For each seed
// whose name is new, a fresh entry is appended with an empty refresh_token, ready
// to be filled by --update-refresh-tokens. A missing file is created. It returns
// the number of entries updated and added.
//
// The edit is done on the YAML node tree, so this never clobbers the working
// refresh tokens of credentials that are already authorized.
func MergeSeedCredentials(filename string, seeds []SeedCredential) (updated, added int, err error) {
	data, err := os.ReadFile(filename)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, 0, fmt.Errorf("failed to read credentials file: %w", err)
	}

	var doc yaml.Node
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return 0, 0, fmt.Errorf("failed to parse credentials file: %w", err)
		}
	}
	root := documentRoot(&doc)

	credsSeq := mappingValue(root, "credentials")
	if credsSeq == nil {
		credsSeq = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "credentials"},
			credsSeq)
	}
	if credsSeq.Kind != yaml.SequenceNode {
		return 0, 0, errors.New("credentials: in file is not a list")
	}

	byName := make(map[string]*yaml.Node, len(credsSeq.Content))
	for _, entry := range credsSeq.Content {
		if entry.Kind != yaml.MappingNode {
			continue
		}
		if n := mappingValue(entry, "name"); n != nil {
			byName[n.Value] = entry
		}
	}

	for _, s := range seeds {
		if entry, ok := byName[s.Name]; ok {
			setMappingStringIfNotEmpty(entry, "key", s.Key)
			setMappingStringIfNotEmpty(entry, "client_id", s.ClientID)
			setMappingStringIfNotEmpty(entry, "client_secret", s.ClientSecret)
			updated++
			continue
		}
		entry := newCredentialNode(s)
		credsSeq.Content = append(credsSeq.Content, entry)
		byName[s.Name] = entry
		added++
	}

	if err := writeFilePreservingMode(filename, &doc); err != nil {
		return 0, 0, err
	}
	return updated, added, nil
}

// documentRoot returns the top-level mapping node of a YAML document, initializing
// an empty document (and its root mapping) in place when the parsed node is empty
// so a missing or blank credentials file can be seeded from scratch.
func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == 0 {
		doc.Kind = yaml.DocumentNode
	}
	if len(doc.Content) == 0 {
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	return doc.Content[0]
}

// newCredentialNode builds a credentials.yaml entry mapping for a seeded
// credential, in the canonical field order. The refresh_token is left as a bare
// empty key (rendered `refresh_token:`, not `refresh_token: ""`) for
// --update-refresh-tokens to fill in later.
func newCredentialNode(s SeedCredential) *yaml.Node {
	scalar := func(v string) *yaml.Node {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
	}
	entry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, kv := range []struct{ key, val string }{
		{"name", s.Name},
		{"key", s.Key},
		{"client_id", s.ClientID},
		{"client_secret", s.ClientSecret},
	} {
		entry.Content = append(entry.Content, scalar(kv.key), scalar(kv.val))
	}
	entry.Content = append(entry.Content,
		scalar("refresh_token"),
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: ""},
	)
	return entry
}

// setMappingStringIfNotEmpty sets key in a mapping only when value is non-empty,
// so a failed scrape (blank field) never overwrites a good stored value.
func setMappingStringIfNotEmpty(m *yaml.Node, key, value string) {
	if value == "" {
		return
	}
	setMappingString(m, key, value)
}

// mappingValue returns the value node for key in a YAML mapping node, or nil.
func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// setMappingString sets key to a string value in a YAML mapping node, updating
// the existing entry if present or appending a new key/value otherwise. The
// value is written as a plain (unquoted) scalar to match the unquoted style of
// credentials.yaml; the encoder still quotes automatically if a value would
// otherwise be ambiguous (e.g. parse back as a number or bool).
func setMappingString(m *yaml.Node, key, value string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1].Kind = yaml.ScalarNode
			m.Content[i+1].Tag = "!!str"
			m.Content[i+1].Style = 0 // plain (unquoted)
			m.Content[i+1].Value = value
			return
		}
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

// writeFilePreservingMode marshals the YAML node and writes it to filename
// atomically (temp file + rename), preserving the original file's permissions.
func writeFilePreservingMode(filename string, doc *yaml.Node) error {
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2) // match the 2-space style used in credentials.yaml.example
	if err := enc.Encode(doc); err != nil {
		return fmt.Errorf("failed to encode credentials file: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("failed to encode credentials file: %w", err)
	}
	out := []byte(buf.String())

	mode := os.FileMode(0600)
	if info, err := os.Stat(filename); err == nil {
		mode = info.Mode().Perm()
	}

	tmp, err := os.CreateTemp(filepath.Dir(filename), ".credentials-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once renamed

	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}
	if err := os.Rename(tmpName, filename); err != nil {
		return fmt.Errorf("failed to replace credentials file: %w", err)
	}
	return nil
}
