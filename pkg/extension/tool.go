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

package extension

import (
	"context"
	"fmt"

	yaml "github.com/goccy/go-yaml"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

// ToolSpec is a tool type an extension adds. Its tools take no source;
// the yaml document has only the common fields (name, type, description,
// authRequired).
type ToolSpec struct {
	Description string // default description
	ReadOnly    bool
	Params      []Param
	Invoke      func(ctx context.Context, args map[string]any) (any, error)
}

// Param is a tool parameter. Type is string, integer, float or boolean.
type Param struct {
	Name        string
	Type        string
	Description string
	Optional    bool
}

// RegisterTool registers a tool type. It panics if the type exists.
func RegisterTool(typ string, spec ToolSpec) {
	ps := parameters.Parameters{}
	for _, p := range spec.Params {
		req := !p.Optional
		switch p.Type {
		case "string":
			ps = append(ps, parameters.NewStringParameter(p.Name, p.Description, parameters.WithStringRequired(req)))
		case "integer":
			ps = append(ps, parameters.NewIntParameter(p.Name, p.Description, parameters.WithIntRequired(req)))
		case "float":
			ps = append(ps, parameters.NewFloatParameter(p.Name, p.Description, parameters.WithFloatRequired(req)))
		case "boolean":
			ps = append(ps, parameters.NewBooleanParameter(p.Name, p.Description, parameters.WithBooleanRequired(req)))
		default:
			panic(fmt.Sprintf("tool type %q: parameter %q has unknown type %q", typ, p.Name, p.Type))
		}
	}
	ok := tools.Register(typ, func(ctx context.Context, name string, d *yaml.Decoder) (tools.ToolConfig, error) {
		c := toolConfig{ConfigBase: tools.ConfigBase{Name: name}, spec: spec, params: ps}
		if err := d.DecodeContext(ctx, &c); err != nil {
			return nil, err
		}
		return c, nil
	})
	if !ok {
		panic(fmt.Sprintf("tool type %q already registered", typ))
	}
}

type toolConfig struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string `yaml:"type" validate:"required"`
	spec             ToolSpec
	params           parameters.Parameters
}

func (c toolConfig) ToolConfigType() string { return c.Type }

func (c toolConfig) Initialize(context.Context) (tools.Tool, error) {
	if c.Description == "" {
		c.Description = c.spec.Description
	}
	_, manifest, err := parameters.ProcessParameters(nil, c.params)
	if err != nil {
		return nil, err
	}
	ann := tools.NewDestructiveAnnotations()
	if c.spec.ReadOnly {
		ann = tools.NewReadOnlyAnnotations()
	}
	if c.Annotations != nil {
		ann = c.Annotations
	}
	m := tools.Manifest{Description: c.Description, Parameters: manifest, AuthRequired: c.AuthRequired}
	return tool{BaseTool: tools.NewBaseTool(c, ann, m, c.params)}, nil
}

type tool struct {
	tools.BaseTool[toolConfig]
}

func (t tool) Invoke(ctx context.Context, _ sources.Source, params parameters.ParamValues, _ tools.AccessToken) (any, util.ToolboxError) {
	out, err := t.Cfg.spec.Invoke(ctx, params.AsMap())
	if err != nil {
		return nil, util.NewAgentError(err.Error(), err)
	}
	return out, nil
}

func (t tool) EmbedParams(_ context.Context, pv parameters.ParamValues, _ tools.PrimitiveManagerI) (parameters.ParamValues, error) {
	return pv, nil
}

func (t tool) GetSourceName() string               { return "" }
func (t tool) ToConfig() tools.ToolConfig          { return t.Cfg }
func (t tool) ValidateSource(sources.Source) error { return nil }
