package main

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
)

type configuration struct {
	Config accessConfig `json:"access,omitempty"`
}

func (c *configuration) Validate() error {
	return c.Config.validate()
}

func (c *configuration) SetDefault() {}

type accessConfig struct {
	// Plugins is a map of repositories (eg "k/k") to lists of plugin names.
	RepoPlugins map[string][]string `json:"repo_plugins,omitempty"`

	// Plugins is a list available plugins.
	Plugins []pluginConfig `json:"plugins,omitempty"`
}

func (a accessConfig) validate() error {
	for i := range a.Plugins {
		if err := a.Plugins[i].validate(); err != nil {
			return err
		}
	}

	ps := make([]string, len(a.Plugins))
	for i := range a.Plugins {
		ps[i] = a.Plugins[i].Name
	}

	total := sets.NewString(ps...)

	for k, item := range a.RepoPlugins {
		if v := sets.NewString(item...).Difference(total); v.Len() != 0 {
			return fmt.Errorf(
				"%s: unknown plugins(%s) are set", k,
				strings.Join(v.UnsortedList(), ", "),
			)
		}
	}

	return nil
}

type eventsDemux map[string][]string

func updateDemux(p *pluginConfig, d eventsDemux) {
	endpoint := p.Endpoint

	for _, e := range p.Events {
		if es, ok := d[e]; ok {
			d[e] = append(es, endpoint)
		} else {
			d[e] = []string{endpoint}
		}
	}
}

func orgOfRepo(repo string) string {
	spliter := "/"
	if strings.Contains(repo, spliter) {
		return strings.Split(repo, spliter)[0]
	}
	return ""
}

func (a accessConfig) getDemux() map[string]eventsDemux {
	plugins := make(map[string]int)
	for i := range a.Plugins {
		plugins[a.Plugins[i].Name] = i
	}

	r := make(map[string]eventsDemux)
	rp := a.RepoPlugins

	for k, ps := range rp {
		events, ok := r[k]
		if !ok {
			events = make(eventsDemux)
			r[k] = events
		}

		// inherit the config of org if k is a repo.
		if org := orgOfRepo(k); org != "" {
			ps = append(ps, rp[org]...)
		}

		for _, p := range ps {
			if i, ok := plugins[p]; ok {
				updateDemux(&a.Plugins[i], events)
			}
		}
	}

	return r
}

type pluginConfig struct {
	// Name of the plugin.
	Name string `json:"name" required:"true"`

	// Endpoint is the location of the plugin.
	Endpoint string `json:"endpoint" required:"true"`

	// Events are the events that this plugin can handle and should be forward to it.
	// If no events are specified, everything is sent.
	Events []string `json:"events,omitempty"`
}

func (p pluginConfig) validate() error {
	if p.Name == "" {
		return fmt.Errorf("missing name")
	}

	if p.Endpoint == "" {
		return fmt.Errorf("missing endpoint")
	}

	// TODO validate the value of p.Endpoint
	return nil
}
