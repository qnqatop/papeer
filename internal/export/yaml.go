package export

import (
	"fmt"
	"io"

	"github.com/qnqatop/papeer/internal/db"
	"github.com/qnqatop/papeer/internal/ptr"

	"gopkg.in/yaml.v3"
)

type yamlFile struct {
	Axes   map[string]yamlAxis `yaml:"axes,omitempty"`
	Topics map[string]yamlAxis `yaml:"topics,omitempty"`
}

type yamlAxis struct {
	Description   string   `yaml:"description"`
	YearMin       int      `yaml:"year_min"`
	MaxPerQuery   int      `yaml:"max_per_query"`
	LangScope     string   `yaml:"lang_scope,omitempty"` // "en" (default) or "ru"
	Queries       []string `yaml:"queries"`
	KeywordsMust  []string `yaml:"keywords_must"`
	KeywordsBoost []string `yaml:"keywords_boost"`
}

// ImportAxesFromYAML reads a YAML file with axes/queries/keywords config
// and returns db.Axis slices ready for SaveAxis.
// Supports both "axes" (legacy) and "topics" (v2) keys.
func ImportAxesFromYAML(r io.Reader, profileID int64) ([]db.Axis, error) {
	var f yamlFile
	if err := yaml.NewDecoder(r).Decode(&f); err != nil {
		return nil, fmt.Errorf("decode yaml: %w", err)
	}

	// Merge "topics" into "axes" for unified processing
	if f.Axes == nil {
		f.Axes = make(map[string]yamlAxis)
	}
	for k, v := range f.Topics {
		if _, exists := f.Axes[k]; !exists {
			f.Axes[k] = v
		}
	}

	axes := make([]db.Axis, 0, len(f.Axes))
	pos := 0
	for key, ya := range f.Axes {
		a := db.Axis{
			ProfileID:   profileID,
			AxisKey:     key,
			Description: ya.Description,
			Position:    pos,
			LangScope:   ya.LangScope, // empty → normalized to "en" by SaveAxis
		}
		if ya.YearMin != 0 {
			a.YearMin = ptr.Ptr(ya.YearMin)
		}
		if ya.MaxPerQuery != 0 {
			a.MaxPerQuery = ptr.Ptr(ya.MaxPerQuery)
		}

		for i, q := range ya.Queries {
			a.Queries = append(a.Queries, db.Query{
				Text:     q,
				Position: i,
			})
		}

		for _, kw := range ya.KeywordsMust {
			a.Keywords = append(a.Keywords, db.Keyword{
				Word: kw,
				Type: "must",
			})
		}
		for _, kw := range ya.KeywordsBoost {
			a.Keywords = append(a.Keywords, db.Keyword{
				Word: kw,
				Type: "boost",
			})
		}

		axes = append(axes, a)
		pos++
	}

	return axes, nil
}

// ExportAxesToYAML конвертирует несколько осей в единый YAML-файл (ключ "topics").
func ExportAxesToYAML(axes []db.Axis) ([]byte, error) {
	f := yamlFile{Topics: make(map[string]yamlAxis, len(axes))}
	for i := range axes {
		f.Topics[axes[i].AxisKey] = axisToYAML(&axes[i])
	}
	return yaml.Marshal(&f)
}

// ExportAxisToYAML конвертирует структуру оси из БД в YAML-формат (ключ "topics").
func ExportAxisToYAML(axis *db.Axis) ([]byte, error) {
	f := yamlFile{
		Topics: map[string]yamlAxis{
			axis.AxisKey: axisToYAML(axis),
		},
	}
	return yaml.Marshal(&f)
}

func axisToYAML(axis *db.Axis) yamlAxis {
	ya := yamlAxis{Description: axis.Description}
	// Only emit lang_scope for non-default (ru) axes to keep English exports clean.
	if axis.LangScope != "" && axis.LangScope != "en" {
		ya.LangScope = axis.LangScope
	}
	if axis.YearMin != nil {
		ya.YearMin = *axis.YearMin
	}
	if axis.MaxPerQuery != nil {
		ya.MaxPerQuery = *axis.MaxPerQuery
	}
	for _, q := range axis.Queries {
		ya.Queries = append(ya.Queries, q.Text)
	}
	for _, k := range axis.Keywords {
		switch k.Type {
		case "must":
			ya.KeywordsMust = append(ya.KeywordsMust, k.Word)
		case "boost":
			ya.KeywordsBoost = append(ya.KeywordsBoost, k.Word)
		}
	}
	return ya
}
