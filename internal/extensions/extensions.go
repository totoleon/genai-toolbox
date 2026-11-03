// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package extensions runs the extensions of pkg/extension inside the
// server: it parses `kind: extension` documents and the `extensions:`
// field of tools, keeps the instances across reloads, hands tool calls to
// them, and serves their HTTP handlers. With no `kind: extension`
// document, nothing here does anything.
package extensions

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/pkg/extension"
)

// Config is a `kind: extension` document.
type Config struct {
	Type   string
	Config map[string]any // the document without kind, name and type
}

// Configs are the extension documents of a configuration, by name.
type Configs map[string]Config

// ParseConfig reads a decoded `kind: extension` document.
func ParseConfig(name string, m map[string]any) (Config, error) {
	typ, _ := m["type"].(string)
	if _, ok := extension.Lookup(typ); !ok {
		return Config{}, fmt.Errorf("extension %q: unknown type %q (is the extension compiled into this binary?)", name, typ)
	}
	cfg := make(map[string]any, len(m))
	for k, v := range m {
		if k != "kind" && k != "type" && k != "name" {
			cfg[k] = v
		}
	}
	return Config{Type: typ, Config: cfg}, nil
}

// ToolConfig is a tool config with an `extensions:` field.
type ToolConfig struct {
	tools.ToolConfig
	Options map[string]map[string]any // instance name -> the tool's options for it
}

// TakeToolOptions removes a tool document's `extensions:` field, so the
// tool's own strict decoding never sees it, and returns the tool config
// wrapped with it. It returns c unchanged when the field is absent.
func TakeToolOptions(tool string, m map[string]any) (func(tools.ToolConfig) tools.ToolConfig, error) {
	raw, ok := m["extensions"]
	if !ok {
		return func(c tools.ToolConfig) tools.ToolConfig { return c }, nil
	}
	delete(m, "extensions")
	byName, ok := raw.(map[string]any)
	if !ok || len(byName) == 0 {
		return nil, fmt.Errorf("tool %q: extensions is a map from extension name to options", tool)
	}
	opts := map[string]map[string]any{}
	for name, v := range byName {
		o, _ := v.(map[string]any)
		if v != nil && o == nil {
			return nil, fmt.Errorf("tool %q: the options for extension %q are a map", tool, name)
		}
		if o == nil {
			o = map[string]any{}
		}
		opts[name] = o
	}
	return func(c tools.ToolConfig) tools.ToolConfig { return ToolConfig{ToolConfig: c, Options: opts} }, nil
}

// live are the running instances, by name. They outlive a reload: an
// instance that implements Reconfigurer gets the new configuration.
var (
	mu   sync.Mutex
	live = map[string]instance{}
)

type instance struct {
	typ string
	v   any
}

// Generation is one initialization of the server's tools: the instances
// and what they see of the server.
type Generation struct {
	ctx       context.Context
	sources   map[string]sources.Source
	tools     map[string]tools.Tool // unwrapped
	instances map[string]any
}

// Start brings the running instances in line with cfgs: it creates new
// ones, reconfigures or replaces changed ones, and closes removed ones.
func Start(ctx context.Context, cfgs Configs, srcs map[string]sources.Source) (*Generation, error) {
	g := &Generation{ctx: ctx, sources: srcs, tools: map[string]tools.Tool{}, instances: map[string]any{}}
	mu.Lock()
	defer mu.Unlock()
	for _, name := range sortedNames(cfgs) {
		c := cfgs[name]
		if in, ok := live[name]; ok && in.typ == c.Type {
			if r, ok := in.v.(extension.Reconfigurer); ok {
				if err := r.Reconfigure(ctx, c.Config, g); err != nil {
					return nil, fmt.Errorf("extension %q: %w", name, err)
				}
				g.instances[name] = in.v
				continue
			}
		}
		if in, ok := live[name]; ok {
			closeInstance(ctx, in)
			delete(live, name)
		}
		f, _ := extension.Lookup(c.Type)
		v, err := f(ctx, name, c.Config, g)
		if err != nil {
			return nil, fmt.Errorf("extension %q: %w", name, err)
		}
		live[name] = instance{typ: c.Type, v: v}
		g.instances[name] = v
	}
	for name, in := range live {
		if _, ok := cfgs[name]; !ok {
			closeInstance(ctx, in)
			delete(live, name)
		}
	}
	return g, nil
}

func sortedNames(cfgs Configs) []string {
	names := make([]string, 0, len(cfgs))
	for n := range cfgs {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func closeInstance(ctx context.Context, in instance) {
	if c, ok := in.v.(extension.Closer); ok {
		_ = c.Close(ctx)
	}
}

// CloseAll closes every running instance, at shutdown.
func CloseAll(ctx context.Context) {
	mu.Lock()
	defer mu.Unlock()
	for name, in := range live {
		closeInstance(ctx, in)
		delete(live, name)
	}
}

// Wrap returns the tool handed to the instances its config names, or the
// tool unchanged.
func (g *Generation) Wrap(name string, tc tools.ToolConfig, t tools.Tool, src sources.Source) (tools.Tool, error) {
	g.tools[name] = t
	ec, ok := tc.(ToolConfig)
	if !ok {
		return t, nil
	}
	for _, by := range sortedOptionNames(ec.Options) {
		v, ok := g.instances[by]
		if !ok {
			return nil, fmt.Errorf("tool %q: no `kind: extension` named %q", name, by)
		}
		i, ok := v.(extension.Interceptor)
		if !ok {
			return nil, fmt.Errorf("tool %q: extension %q does not take tool calls", name, by)
		}
		t = wrappedTool{Tool: t, name: name, by: i, options: ec.Options[by]}
	}
	return t, nil
}

func sortedOptionNames(m map[string]map[string]any) []string {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Source implements extension.Host.
func (g *Generation) Source(name string) (any, bool) {
	s, ok := g.sources[name]
	return s, ok
}

// HTTPHandler serves /extensions/<name>/... from the running instance
// named <name>, if it implements extension.HTTPHandler.
func HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/extensions/")
		name, _, _ := strings.Cut(rest, "/")
		mu.Lock()
		in, ok := live[name]
		mu.Unlock()
		h, isHandler := in.v.(extension.HTTPHandler)
		if !ok || !isHandler {
			http.NotFound(w, r)
			return
		}
		prefix := "/extensions/" + name
		if r.URL.Path == prefix {
			http.Redirect(w, r, prefix+"/", http.StatusMovedPermanently)
			return
		}
		http.StripPrefix(prefix, h.HTTPHandler()).ServeHTTP(w, r)
	})
}
