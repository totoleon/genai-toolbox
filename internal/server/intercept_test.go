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

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/postgres/postgressql"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/googleapis/mcp-toolbox/pkg/extension"
)

// recorder is an interceptor that records the calls it sees and lets them
// through.
type recorder struct{ calls []extension.Call }

func (r *recorder) Intercept(ctx context.Context, c extension.Call, next extension.Next) (any, error) {
	r.calls = append(r.calls, c)
	return next(ctx)
}

var rec = &recorder{}

func init() {
	extension.RegisterInterceptor("test-recorder", func(context.Context, string, map[string]any, extension.Host) (extension.Interceptor, error) {
		return rec, nil
	})
}

const twoTools = `
kind: tool
name: cancel
type: postgres-sql
source: db
description: Cancel an order.
parameters:
  - {name: id, type: integer, description: The order.}
statement: UPDATE orders SET status = 'cancelled' WHERE id = $1
%s
---
kind: tool
name: list
type: postgres-sql
source: db
description: List orders.
statement: SELECT * FROM orders
`

func parseTools(t *testing.T, yaml string) ToolConfigs {
	t.Helper()
	_, _, _, tcs, _, _, _, _, err := UnmarshalPrimitiveConfig(context.Background(), []byte(yaml))
	if err != nil {
		t.Fatalf("UnmarshalPrimitiveConfig: %v", err)
	}
	return tcs
}

func TestNoInterceptorLeavesToolsAsTheyAre(t *testing.T) {
	tcs := parseTools(t, fmt.Sprintf(twoTools, ""))
	for name, tc := range tcs {
		if _, ok := tc.(postgressql.Config); !ok {
			t.Errorf("tool %s: config is %T, want postgressql.Config", name, tc)
		}
	}
}

func TestInterceptField(t *testing.T) {
	yaml := "kind: interceptor\nname: review\ntype: test-recorder\nlevel: 3\n---\n" +
		fmt.Sprintf(twoTools, "intercept: {by: review, policy: writes}")
	tcs := parseTools(t, yaml)
	ic, ok := tcs["cancel"].(interceptedConfig)
	if !ok {
		t.Fatalf("cancel: config is %T, want interceptedConfig", tcs["cancel"])
	}
	if ic.by != "review" || !cmp.Equal(ic.options, map[string]any{"policy": "writes"}) {
		t.Errorf("cancel: by %q options %v", ic.by, ic.options)
	}
	if _, ok := ic.ToolConfig.(postgressql.Config); !ok {
		t.Errorf("cancel: inner config is %T", ic.ToolConfig)
	}
	if _, ok := tcs["list"].(postgressql.Config); !ok {
		t.Errorf("list: a tool without intercept is %T, want postgressql.Config", tcs["list"])
	}
	interceptorMu.Lock()
	got := interceptorConfigs["review"]
	interceptorMu.Unlock()
	if got.Type != "test-recorder" || !cmp.Equal(got.Config, map[string]any{"level": uint64(3)}) {
		t.Errorf("interceptor config: %+v", got)
	}
}

func TestUnknownInterceptorType(t *testing.T) {
	_, _, _, _, _, _, _, _, err := UnmarshalPrimitiveConfig(context.Background(), []byte("kind: interceptor\nname: x\ntype: nope\n"))
	if err == nil {
		t.Fatal("want an error for an interceptor type no extension registered")
	}
}

// fakeTool runs nothing; it returns "ran".
type fakeTool struct {
	tools.Tool
	cfg tools.ToolConfig
}

func (f fakeTool) Invoke(context.Context, sources.Source, parameters.ParamValues, tools.AccessToken) (any, util.ToolboxError) {
	return "ran", nil
}
func (f fakeTool) ToConfig() tools.ToolConfig { return f.cfg }
func (f fakeTool) GetSourceName() string      { return "db" }
func (f fakeTool) GetDescription() string     { return "Cancel an order." }

func TestInterceptedToolSeesRenderedSQL(t *testing.T) {
	interceptorMu.Lock()
	interceptorConfigs["review"] = interceptorConfig{Type: "test-recorder"}
	interceptorMu.Unlock()
	tcs := parseTools(t, fmt.Sprintf(twoTools, "intercept: review"))
	in := newInterception(context.Background(), nil)
	ic := tcs["cancel"].(interceptedConfig)
	wrapped, err := in.wrap("cancel", ic, fakeTool{cfg: ic.ToolConfig}, nil)
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := in.wrap("list", tcs["list"], fakeTool{cfg: tcs["list"]}, nil)
	if _, ok := plain.(fakeTool); !ok {
		t.Errorf("list: wrapped as %T, want the tool itself", plain)
	}

	rec.calls = nil
	out, terr := wrapped.Invoke(context.Background(), nil, parameters.ParamValues{{Name: "id", Value: 7}}, "")
	if terr != nil || out != "ran" {
		t.Fatalf("Invoke = %v, %v; want ran (the interceptor calls next)", out, terr)
	}
	want := extension.Call{
		Tool: "cancel", ToolType: "postgres-sql", Description: "Cancel an order.", Source: "db",
		Args: map[string]any{"id": 7}, Options: map[string]any{},
		Statements: []extension.Statement{{SQL: "UPDATE orders SET status = 'cancelled' WHERE id = $1", Args: []extension.Arg{{Type: "integer", Value: 7}}}},
	}
	if len(rec.calls) != 1 {
		t.Fatalf("interceptor saw %d calls, want 1", len(rec.calls))
	}
	if diff := cmp.Diff(want, rec.calls[0]); diff != "" {
		t.Errorf("call (-want +got):\n%s", diff)
	}
}
