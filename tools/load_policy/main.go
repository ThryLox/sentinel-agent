package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"sentinel-agent/internal/config"
	"sentinel-agent/internal/policy"
)

type YPolicy struct {
	ID    string           `yaml:"id"`
	Name  string           `yaml:"name"`
	Rules []map[string]any `yaml:"rules"`
}

type YDoc struct {
	Version  int       `yaml:"version"`
	Policies []YPolicy `yaml:"policies"`
}

func main() {
	yamlPath := flag.String("f", "policies.yaml", "path to policies YAML")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load config:", err)
		os.Exit(1)
	}

	b, err := ioutil.ReadFile(*yamlPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read policy file:", err)
		os.Exit(1)
	}
	var doc YDoc
	if err := yaml.Unmarshal(b, &doc); err != nil {
		fmt.Fprintln(os.Stderr, "yaml parse error:", err)
		os.Exit(1)
	}

	ps, err := policy.NewDBStore(cfg.DBPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open policy db:", err)
		os.Exit(1)
	}
	defer ps.Close()

	for _, p := range doc.Policies {
		j, _ := json.Marshal(map[string]any{
			"version": doc.Version,
			"id":      p.ID,
			"name":    p.Name,
			"rules":   p.Rules,
		})
		pol := &policy.Policy{
			ID:      p.ID,
			Name:    p.Name,
			Raw:     string(j),
			Updated: time.Now().UTC(),
		}
		if err := ps.Set(pol); err != nil {
			fmt.Fprintf(os.Stderr, "failed to set policy %s: %v\n", p.ID, err)
		} else {
			fmt.Println("stored policy", p.ID)
		}
	}
}
