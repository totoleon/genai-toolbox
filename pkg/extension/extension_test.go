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

package extension_test

import (
	"github.com/googleapis/mcp-toolbox/internal/sources/alloydbpg"
	"github.com/googleapis/mcp-toolbox/internal/sources/bigquery"
	"github.com/googleapis/mcp-toolbox/internal/sources/cloudsqlmysql"
	"github.com/googleapis/mcp-toolbox/internal/sources/cloudsqlpg"
	"github.com/googleapis/mcp-toolbox/internal/sources/mysql"
	"github.com/googleapis/mcp-toolbox/internal/sources/postgres"
	"github.com/googleapis/mcp-toolbox/internal/sources/spanner"
	"github.com/googleapis/mcp-toolbox/pkg/extension"
)

// The sources extensions reach through these interfaces. A source that
// renames a method fails to compile here.
var (
	_ extension.PostgresPooler   = (*postgres.Source)(nil)
	_ extension.PostgresPooler   = (*alloydbpg.Source)(nil)
	_ extension.PostgresPooler   = (*cloudsqlpg.Source)(nil)
	_ extension.MySQLPooler      = (*mysql.Source)(nil)
	_ extension.MySQLPooler      = (*cloudsqlmysql.Source)(nil)
	_ extension.SpannerClienter  = (*spanner.Source)(nil)
	_ extension.BigQueryClienter = (*bigquery.Source)(nil)
)
