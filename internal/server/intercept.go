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

package server

// Interceptors from extensions (pkg/extension). A `kind: interceptor`
// document configures one; a tool with an `intercept:` field goes through
// it. Without either, nothing here runs.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/bigquery/bigqueryexecutesql"
	"github.com/googleapis/mcp-toolbox/internal/tools/bigquery/bigquerysql"
	"github.com/googleapis/mcp-toolbox/internal/tools/mysql/mysqlexecutesql"
	"github.com/googleapis/mcp-toolbox/internal/tools/mysql/mysqlsql"
	"github.com/googleapis/mcp-toolbox/internal/tools/postgres/postgresexecutesql"
	"github.com/googleapis/mcp-toolbox/internal/tools/postgres/postgressql"
	"github.com/googleapis/mcp-toolbox/internal/tools/spanner/spannerexecutesql"
	"github.com/googleapis/mcp-toolbox/internal/tools/spanner/spannersql"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/googleapis/mcp-toolbox/pkg/extension"
)

// interceptorConfig is a `kind: interceptor` document.
type interceptorConfig struct {
	Type   string
	Config map[string]any
}

// interceptorConfigs holds the interceptor documents parsed so far, by
// name. Parsing fills it; tool initialization reads it.
var (
	interceptorMu      sync.Mutex
	interceptorConfigs = map[string]interceptorConfig{}
)

func addInterceptorConfig(name string, m map[string]any) error {
	typ, _ := m["type"].(string)
	if _, ok := extension.LookupInterceptor(typ); !ok {
		return fmt.Errorf("interceptor %q: unknown type %q (is the extension compiled in?)", name, typ)
	}
	cfg := make(map[string]any, len(m))
	for k, v := range m {
		if k != "type" && k != "name" {
			cfg[k] = v
		}
	}
	interceptorMu.Lock()
	defer interceptorMu.Unlock()
	interceptorConfigs[name] = interceptorConfig{Type: typ, Config: cfg}
	return nil
}

// interceptedConfig is a tool config with an `intercept:` field.
type interceptedConfig struct {
	tools.ToolConfig
	by      string
	options map[string]any
}

// takeIntercept removes a tool document's `intercept:` field, so the
// tool's own strict decoding never sees it. It accepts the interceptor's
// name, or a map with `by` and the interceptor's options.
func takeIntercept(tool string, m map[string]any) (*interceptedConfig, error) {
	raw, ok := m["intercept"]
	if !ok {
		return nil, nil
	}
	delete(m, "intercept")
	ic := &interceptedConfig{options: map[string]any{}}
	switch v := raw.(type) {
	case string:
		ic.by = v
	case map[string]any:
		for k, x := range v {
			if k == "by" {
				ic.by, _ = x.(string)
			} else {
				ic.options[k] = x
			}
		}
	default:
		return nil, fmt.Errorf("tool %q: intercept is an interceptor name or a map with `by`", tool)
	}
	if ic.by == "" {
		return nil, fmt.Errorf("tool %q: intercept needs `by`, the interceptor's name", tool)
	}
	return ic, nil
}

// interception is the state of one initializeTools run: the interceptors
// built so far and what they see of the server.
type interception struct {
	ctx          context.Context
	sources      map[string]sources.Source
	tools        map[string]tools.Tool // unwrapped, filled when all tools are up
	interceptors map[string]extension.Interceptor
}

func newInterception(ctx context.Context, srcs map[string]sources.Source) *interception {
	return &interception{ctx: ctx, sources: srcs, tools: map[string]tools.Tool{}, interceptors: map[string]extension.Interceptor{}}
}

// wrap returns the tool behind its interceptor, or the tool unchanged.
func (in *interception) wrap(name string, tc tools.ToolConfig, t tools.Tool, src sources.Source) (tools.Tool, error) {
	in.tools[name] = t
	ic, ok := tc.(interceptedConfig)
	if !ok {
		return t, nil
	}
	i, err := in.interceptor(ic.by)
	if err != nil {
		return nil, fmt.Errorf("tool %q: %w", name, err)
	}
	return interceptedTool{Tool: t, name: name, src: src, by: i, options: ic.options}, nil
}

func (in *interception) interceptor(name string) (extension.Interceptor, error) {
	if i, ok := in.interceptors[name]; ok {
		return i, nil
	}
	interceptorMu.Lock()
	c, ok := interceptorConfigs[name]
	interceptorMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no `kind: interceptor` named %q", name)
	}
	f, _ := extension.LookupInterceptor(c.Type)
	i, err := f(in.ctx, name, c.Config, in)
	if err != nil {
		return nil, fmt.Errorf("interceptor %q: %w", name, err)
	}
	in.interceptors[name] = i
	return i, nil
}

// Source implements extension.Host.
func (in *interception) Source(name string) (any, bool) {
	s, ok := in.sources[name]
	return s, ok
}

// Invoke implements extension.Host.
func (in *interception) Invoke(ctx context.Context, name string, args map[string]any) (any, error) {
	t, ok := in.tools[name]
	if !ok {
		return nil, fmt.Errorf("no tool %q", name)
	}
	src := in.sources[t.GetSourceName()]
	ps, err := t.GetParameters(src)
	if err != nil {
		return nil, err
	}
	params, err := parameters.ParseParams(ps, asNumbers(args), nil)
	if err != nil {
		return nil, err
	}
	out, terr := t.Invoke(ctx, src, params, "")
	if terr != nil {
		return nil, terr
	}
	return out, nil
}

type interceptedTool struct {
	tools.Tool
	name    string
	src     sources.Source
	by      extension.Interceptor
	options map[string]any
}

func (t interceptedTool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, token tools.AccessToken) (any, util.ToolboxError) {
	c := extension.Call{
		Tool: t.name, ToolType: t.ToConfig().ToolConfigType(), Description: t.GetDescription(),
		Source: t.GetSourceName(), Args: params.AsMap(), Options: t.options,
	}
	if s != nil {
		c.SourceType = s.SourceType()
	}
	if st, ok, err := renderStatement(t.ToConfig(), params); err != nil {
		return nil, util.NewAgentError(err.Error(), err)
	} else if ok {
		c.Statements = []extension.Statement{st}
	}
	var terr util.ToolboxError
	out, err := t.by.Intercept(ctx, c, func(ctx context.Context) (any, error) {
		var out any
		out, terr = t.Tool.Invoke(ctx, s, params, token)
		if terr != nil {
			return nil, terr
		}
		return out, nil
	})
	if terr != nil {
		// The tool's own error, passed through.
		return nil, terr
	}
	if err != nil {
		return nil, util.NewClientServerError(err.Error(), http.StatusInternalServerError, err)
	}
	return out, nil
}

// renderStatement is the SQL a tool would run, for the SQL tools that
// take a statement from their configuration or an `sql` argument.
func renderStatement(tc tools.ToolConfig, params parameters.ParamValues) (extension.Statement, bool, error) {
	pm := params.AsMap()
	positional := func(tp parameters.Parameters, ps parameters.Parameters, stmt string) (extension.Statement, bool, error) {
		q, err := parameters.ResolveTemplateParams(tp, stmt, pm)
		if err != nil {
			return extension.Statement{}, false, err
		}
		vals, err := parameters.GetParams(ps, pm)
		if err != nil {
			return extension.Statement{}, false, err
		}
		st := extension.Statement{SQL: q}
		for i, v := range vals.AsSlice() {
			st.Args = append(st.Args, extension.Arg{Type: ps[i].GetType(), Value: v})
		}
		return st, true, nil
	}
	named := func(tp parameters.Parameters, ps parameters.Parameters, stmt string) (extension.Statement, bool, error) {
		q, err := parameters.ResolveTemplateParams(tp, stmt, pm)
		if err != nil {
			return extension.Statement{}, false, err
		}
		st := extension.Statement{SQL: q}
		for _, p := range ps {
			st.Args = append(st.Args, extension.Arg{Name: p.GetName(), Type: p.GetType(), Value: pm[p.GetName()]})
		}
		return st, true, nil
	}
	switch c := tc.(type) {
	case postgressql.Config:
		return positional(c.TemplateParameters, c.Parameters, c.Statement)
	case mysqlsql.Config:
		return positional(c.TemplateParameters, c.Parameters, c.Statement)
	case spannersql.Config:
		return named(c.TemplateParameters, c.Parameters, c.Statement)
	case bigquerysql.Config:
		return named(c.TemplateParameters, c.Parameters, c.Statement)
	case postgresexecutesql.Config, mysqlexecutesql.Config, spannerexecutesql.Config, bigqueryexecutesql.Config:
		q, _ := pm["sql"].(string)
		if strings.TrimSpace(q) == "" {
			return extension.Statement{}, false, fmt.Errorf("sql is empty")
		}
		return extension.Statement{SQL: q}, true, nil
	}
	return extension.Statement{}, false, nil
}

// asNumbers decodes arguments the way the MCP server does (UseNumber), so
// an integer that went through JSON is an integer again.
func asNumbers(args map[string]any) map[string]any {
	b, err := json.Marshal(args)
	if err != nil {
		return args
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var out map[string]any
	if d.Decode(&out) != nil {
		return args
	}
	return out
}
