package query

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/octobud-hq/octobud/backend/internal/db"
)

type viewCatalog struct {
	views []db.View
	loads int
}

func (c *viewCatalog) ListViews(_ context.Context, _ string) ([]db.View, error) {
	c.loads++
	return c.views, nil
}

func namedView(name, filter string) db.View {
	return db.View{ID: name, Name: name, Query: sql.NullString{String: filter, Valid: true}}
}

func TestViewCompiler(t *testing.T) {
	catalog := &viewCatalog{views: []db.View{
		namedView("Noise", `author:bot`),
		namedView("Active", `in:inbox,filtered`),
		namedView("Actionable", `in:"Active" -in:"Noise"`),
	}}
	compiler := NewViewCompiler(context.Background(), "owner", catalog)
	compiled, err := compiler.BuildQuery(`in:"Actionable" OR in:"Noise"`, 2, 8, true)
	require.NoError(t, err)
	require.Equal(t, 1, catalog.loads)
	require.Equal(t, int32(2), compiled.Limit)
	require.Equal(t, int32(8), compiled.Offset)
	where := strings.Join(compiled.Where, " AND ")
	require.Contains(t, where, "n.id IN (SELECT n.id FROM notifications n")
	require.Contains(t, where, "NOT (n.id IN")
	require.NotContains(t, where, "LIMIT")
	require.Equal(
		t,
		[]interface{}{"owner", "%bot%", "owner", "owner", "%bot%", "owner"},
		compiled.Args,
	)
	require.Contains(t, where, "n.muted = 0")

	evaluator, err := compiler.NewEvaluator(`in:"Actionable"`)
	require.NoError(t, err)
	require.True(
		t,
		evaluator.Matches(
			&db.Notification{AuthorLogin: sql.NullString{String: "human", Valid: true}},
			nil,
		),
	)
	require.False(
		t,
		evaluator.Matches(
			&db.Notification{AuthorLogin: sql.NullString{String: "bot", Valid: true}},
			nil,
		),
	)
	require.False(t, evaluator.Matches(&db.Notification{Archived: true}, nil))
}

func TestViewCompilerValidation(t *testing.T) {
	for _, tt := range []struct {
		name  string
		views []db.View
		want  error
	}{
		{"missing", []db.View{namedView("A", `in:"Missing"`)}, ErrViewReferenceNotFound},
		{"self", []db.View{namedView("A", `in:"A"`)}, ErrViewReferenceCycle},
		{"indirect", []db.View{namedView("A", `-in:"B"`), namedView("B", `in:"A"`)}, ErrViewReferenceCycle},
		{"ambiguous", []db.View{namedView("A", `in:inbox`), namedView("a", `in:archive`)}, ErrAmbiguousViewReference},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateViewDefinitions("owner", tt.views)
			require.ErrorIs(t, err, tt.want)
		})
	}
	_, err := ParseAndValidate(`in:"Noise"`)
	require.Error(t, err, "rule-owned queries remain strict")
}

func TestViewCompilerPreservesDefaults(t *testing.T) {
	catalog := &viewCatalog{views: []db.View{
		namedView("Empty", ""),
		namedView("Muted", "is:muted"),
		namedView("Everything", "in:anywhere"),
	}}
	compiler := NewViewCompiler(context.Background(), "owner", catalog)
	empty, err := compiler.BuildQuery(`in:"Empty"`, 1, 0, false)
	require.NoError(t, err)
	require.Contains(t, strings.Join(empty.Where, ""), "n.filtered = 0")
	all, err := compiler.BuildQuery(`in:"Everything"`, 1, 0, false)
	require.NoError(t, err)
	require.NotContains(t, strings.Join(all.Where, ""), "n.muted = 0")
	evaluator, err := compiler.NewEvaluator(`in:"Muted"`)
	require.NoError(t, err)
	require.True(t, evaluator.Matches(&db.Notification{Muted: true}, nil))
}

func TestRewriteViewReferences(t *testing.T) {
	for _, tt := range []struct{ input, old, name, want string }{
		{`in:"Noise" -in:noise title:"Noise"`, "Noise", "Low priority", `in:"Low priority" -in:"Low priority" title:"Noise"`},
		{`(in:inbox,"Noise",filtered OR in:"Noise")  `, "Noise", `A "quote"\path`, `(in:inbox,"A \"quote\"\\path",filtered OR in:"A \"quote\"\\path")  `},
		{`in:"💬 Engaged" AND title:"💬 Engaged"`, "💬 Engaged", "Révision", `in:"Révision" AND title:"💬 Engaged"`},
		{`in:inbox -in:"Noisy" AND "Noise"`, "Noise", "Low", `in:inbox -in:"Noisy" AND "Noise"`},
	} {
		t.Run(tt.input, func(t *testing.T) {
			result, err := RewriteViewReferences(tt.input, tt.old, tt.name)
			require.NoError(t, err)
			require.Equal(t, tt.want, result)
			references, err := ViewReferences(result)
			require.NoError(t, err)
			if strings.Contains(tt.want, `in:"Révision"`) {
				require.Equal(t, []string{"Révision"}, references)
			}
		})
	}
	_, err := RewriteViewReferences(`in:"broken`, "broken", "fixed")
	require.Error(t, err)
}

func TestViewCompilerPropagatesLookupFailure(t *testing.T) {
	want := errors.New("database unavailable")
	compiler := &ViewCompiler{load: func() ([]db.View, error) { return nil, want }}
	_, err := compiler.BuildQuery(`in:"Noise"`, 1, 0, false)
	require.ErrorIs(t, err, want)
}

func TestViewReferenceHintsDoNotNegateUnknownMetadata(t *testing.T) {
	catalog := &viewCatalog{views: []db.View{
		namedView("Noise", `in:inbox,filtered tags:noise`),
		namedView("Actionable", `in:inbox,filtered -in:"Noise"`),
	}}
	compiler := NewViewCompiler(t.Context(), "owner", catalog)
	evaluator, err := compiler.NewEvaluator(`in:"Actionable"`)
	require.NoError(t, err)
	hints := ComputeActionHintsWithEvaluator(&db.Notification{}, nil, evaluator)
	require.ElementsMatch(t, []string{"archive", "mute", "snooze"}, hints.DismissedOn)
}
