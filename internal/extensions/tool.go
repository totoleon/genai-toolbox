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

package extensions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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

// Invoke implements extension.Host.
func (g *Generation) Invoke(ctx context.Context, name string, args map[string]any) (any, error) {
	t, ok := g.tools[name]
	if !ok {
		return nil, fmt.Errorf("no tool %q", name)
	}
	src := g.sources[t.GetSourceName()]
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

// wrappedTool hands a tool's calls to an extension.Interceptor.
type wrappedTool struct {
	tools.Tool
	name    string
	by      extension.Interceptor
	options map[string]any
}

func (t wrappedTool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, token tools.AccessToken) (any, util.ToolboxError) {
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
