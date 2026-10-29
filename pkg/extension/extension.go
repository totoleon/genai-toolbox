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

// Package extension lets a Go module outside this repository add
// experimental features to Toolbox: an interceptor that runs around tool
// calls, and tool types of its own. The module registers them in init()
// and is compiled into a Toolbox binary whose main imports it:
//
//	import (
//		"github.com/googleapis/mcp-toolbox/cmd"
//		_ "example.com/my-extension"
//	)
//
//	func main() { cmd.Execute() }
//
// Nothing changes for a configuration that does not use an extension.
// An interceptor is enabled by a `kind: interceptor` document in
// tools.yaml, and only tools with an `intercept:` field go through it:
//
//	kind: interceptor
//	name: review
//	type: my-interceptor     # the type the extension registered
//	...                      # the rest is the extension's configuration
//	---
//	kind: tool
//	name: cancel-order
//	type: postgres-sql
//	intercept: {by: review, policy: order-writes}
//
// EXPERIMENTAL: this API may change in any release.
package extension

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/spanner"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Statement is a SQL statement a tool would run, with its arguments bound
// the way the tool binds them: positionally ($1, ?) when Name is empty,
// by name (@name) otherwise.
type Statement struct {
	SQL  string
	Args []Arg
}

// Arg is one bound argument. Type is the tool parameter's type, when known.
type Arg struct {
	Name  string
	Type  string
	Value any
}

// Call is one tool call an interceptor is asked about.
type Call struct {
	Tool        string
	ToolType    string
	Description string
	Source      string // the tool's source, if it has one
	SourceType  string
	Args        map[string]any
	// Statements is the SQL the tool would run, rendered from its
	// configuration and Args, for the SQL tools Toolbox can render; nil
	// for other tools.
	Statements []Statement
	// Options is the tool's `intercept:` field, without `by`.
	Options map[string]any
}

// Next runs the call as Toolbox would without the interceptor.
type Next func(ctx context.Context) (any, error)

// Interceptor runs around the calls of the tools that name it. It may call
// next, return its own result without calling next, or return an error.
type Interceptor interface {
	Intercept(ctx context.Context, c Call, next Next) (any, error)
}

// Host is the Toolbox server an interceptor runs in.
type Host interface {
	// Source returns an initialized source. Use the interfaces below
	// (PostgresPooler and so on) to reach its connection pool or client.
	Source(name string) (any, bool)
	// Invoke runs a tool with the given arguments, without the
	// interceptor: for a call the interceptor held and now lets through.
	Invoke(ctx context.Context, tool string, args map[string]any) (any, error)
}

// InterceptorFactory builds an interceptor from its `kind: interceptor`
// document (without kind and name). Toolbox calls it at startup and on
// every reload of tools.yaml; an extension that keeps state across reloads
// keeps it itself.
type InterceptorFactory func(ctx context.Context, name string, config map[string]any, host Host) (Interceptor, error)

// The connection pools and clients of the SQL sources. Each source type
// that implements one asserts it in a test, so a rename breaks the build
// here, not an extension.
type (
	PostgresPooler  interface{ PostgresPool() *pgxpool.Pool }
	MySQLPooler     interface{ MySQLPool() *sql.DB }
	SpannerClienter interface {
		SpannerClient() *spanner.Client
		DatabaseDialect() string
	}
	BigQueryClienter interface{ BigQueryClient() *bigquery.Client }
)

var (
	mu           sync.RWMutex
	interceptors = map[string]InterceptorFactory{}
)

// RegisterInterceptor registers an interceptor type. It panics if the type
// is already registered.
func RegisterInterceptor(typ string, f InterceptorFactory) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := interceptors[typ]; dup {
		panic(fmt.Sprintf("interceptor type %q already registered", typ))
	}
	interceptors[typ] = f
}

// LookupInterceptor returns the factory for an interceptor type.
func LookupInterceptor(typ string) (InterceptorFactory, bool) {
	mu.RLock()
	defer mu.RUnlock()
	f, ok := interceptors[typ]
	return f, ok
}
