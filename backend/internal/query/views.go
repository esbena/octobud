package query

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/octobud-hq/octobud/backend/internal/db"
	"github.com/octobud-hq/octobud/backend/internal/query/eval"
	"github.com/octobud-hq/octobud/backend/internal/query/parse"
)

var (
	ErrViewReferenceNotFound  = errors.New("referenced view not found")
	ErrViewReferenceCycle     = errors.New("cyclic view reference")
	ErrAmbiguousViewReference = errors.New("ambiguous view reference")
)

type ViewStore interface {
	ListViews(context.Context, string) ([]db.View, error)
}

// ViewCompiler resolves a single operation against one user's saved definitions.
// Built-in-only queries do not load the catalog.
type ViewCompiler struct {
	userID     string
	load       func() ([]db.View, error)
	views      map[string]db.View
	queries    map[string]db.NotificationQuery
	evaluators map[string]*eval.Evaluator
	stack      []string
}

func NewViewCompiler(ctx context.Context, userID string, store ViewStore) *ViewCompiler {
	return &ViewCompiler{
		userID:     userID,
		load:       func() ([]db.View, error) { return store.ListViews(ctx, userID) },
		queries:    make(map[string]db.NotificationQuery),
		evaluators: make(map[string]*eval.Evaluator),
	}
}

func (c *ViewCompiler) BuildQuery(
	queryStr string, limit, offset int32, includeSubject bool,
) (db.NotificationQuery, error) {
	return buildQuery(queryStr, limit, offset, includeSubject, c.resolveQuery)
}

func (c *ViewCompiler) findView(name string) (db.View, error) {
	if c.views == nil {
		views, err := c.load()
		if err != nil {
			return db.View{}, fmt.Errorf("load referenced views: %w", err)
		}
		catalog := make(map[string]db.View, len(views))
		for _, view := range views {
			key := normalizeViewName(view.Name)
			if _, exists := catalog[key]; exists {
				return db.View{}, fmt.Errorf("%w: %q", ErrAmbiguousViewReference, view.Name)
			}
			catalog[key] = view
		}
		c.views = catalog
	}
	view, ok := c.views[normalizeViewName(name)]
	if !ok {
		return db.View{}, fmt.Errorf("%w: %q", ErrViewReferenceNotFound, name)
	}
	return view, nil
}

func (c *ViewCompiler) enter(view db.View) error {
	key := normalizeViewName(view.Name)
	for _, ancestor := range c.stack {
		if ancestor == key {
			return fmt.Errorf("%w: %s", ErrViewReferenceCycle,
				strings.Join(append(append([]string{}, c.stack...), key), " -> "))
		}
	}
	c.stack = append(c.stack, key)
	return nil
}

func (c *ViewCompiler) resolveQuery(name string) (db.NotificationQuery, error) {
	view, err := c.findView(name)
	if err != nil {
		return db.NotificationQuery{}, err
	}
	key := normalizeViewName(view.Name)
	if compiled, ok := c.queries[key]; ok {
		return compiled, nil
	}
	if enterErr := c.enter(view); enterErr != nil {
		return db.NotificationQuery{}, enterErr
	}
	defer func() { c.stack = c.stack[:len(c.stack)-1] }()

	compiled, err := c.BuildQuery(view.Query.String, 0, 0, false)
	if err != nil {
		return db.NotificationQuery{}, fmt.Errorf("view %q: %w", view.Name, err)
	}
	compiled.Where = append(compiled.Where, "n.user_id = ?")
	compiled.Args = append(compiled.Args, c.userID)
	c.queries[key] = compiled
	return compiled, nil
}

func (c *ViewCompiler) NewEvaluator(queryStr string) (*eval.Evaluator, error) {
	ast, err := parseAndValidate(queryStr, true)
	if err != nil {
		return nil, err
	}
	evaluator := eval.NewEvaluator(ast)
	evaluator.Views = make(map[string]*eval.Evaluator)
	for _, name := range referencesInNode(ast) {
		view, err := c.findView(name)
		if err != nil {
			return nil, err
		}
		key := normalizeViewName(view.Name)
		child, ok := c.evaluators[key]
		if !ok {
			if enterErr := c.enter(view); enterErr != nil {
				return nil, enterErr
			}
			child, err = c.NewEvaluator(view.Query.String)
			c.stack = c.stack[:len(c.stack)-1]
			if err != nil {
				return nil, err
			}
			c.evaluators[key] = child
		}
		evaluator.Views[normalizeViewName(name)] = child
	}
	return evaluator, nil
}

// ValidateViewDefinitions includes prospective edits before they are persisted.
func ValidateViewDefinitions(userID string, views []db.View) error {
	compiler := &ViewCompiler{
		userID:  userID,
		load:    func() ([]db.View, error) { return views, nil },
		queries: make(map[string]db.NotificationQuery),
	}
	for _, view := range views {
		if _, err := compiler.resolveQuery(view.Name); err != nil {
			return err
		}
	}
	return nil
}

func ViewReferences(queryStr string) ([]string, error) {
	ast, err := parseAndValidate(queryStr, true)
	if err != nil {
		return nil, err
	}
	return referencesInNode(ast), nil
}

func referencesInNode(node parse.Node) []string {
	var names []string
	var visit func(parse.Node)
	visit = func(node parse.Node) {
		switch n := node.(type) {
		case *parse.Term:
			if strings.EqualFold(n.Field, "in") {
				for _, value := range n.Values {
					if !parse.IsBuiltinInValue(value) {
						names = append(names, value)
					}
				}
			}
		case *parse.BinaryExpr:
			visit(n.Left)
			visit(n.Right)
		case *parse.NotExpr:
			visit(n.Expr)
		case *parse.ParenExpr:
			visit(n.Expr)
		}
	}
	visit(node)
	return names
}

func ReferencesView(queryStr, name string) (bool, error) {
	names, err := ViewReferences(queryStr)
	if err != nil {
		return false, err
	}
	for _, reference := range names {
		if normalizeViewName(reference) == normalizeViewName(name) {
			return true, nil
		}
	}
	return false, nil
}

// RewriteViewReferences edits operand spans, leaving unrelated source text intact.
func RewriteViewReferences(queryStr, oldName, newName string) (string, error) {
	if _, err := ViewReferences(queryStr); err != nil {
		return "", err
	}
	tokens, err := parse.NewLexer(queryStr).Tokenize()
	if err != nil {
		return "", err
	}
	replacement := `"` + strings.NewReplacer(
		`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`,
	).Replace(newName) + `"`
	var result strings.Builder
	last := 0
	for i := 0; i+2 < len(tokens); i++ {
		if tokens[i].Type != parse.TokenFreeText ||
			!strings.EqualFold(tokens[i].Value, "in") ||
			tokens[i+1].Type != parse.TokenColon {
			continue
		}
		i += 2
		for {
			token := tokens[i]
			if !parse.IsBuiltinInValue(token.Value) &&
				normalizeViewName(token.Value) == normalizeViewName(oldName) {
				start := token.Pos - 1
				end := start + len(strings.TrimRightFunc(
					queryStr[start:tokens[i+1].Pos-1], unicode.IsSpace))
				result.WriteString(queryStr[last:start])
				result.WriteString(replacement)
				last = end
			}
			if tokens[i+1].Type != parse.TokenComma {
				break
			}
			i += 2
		}
	}
	result.WriteString(queryStr[last:])
	return result.String(), nil
}

func normalizeViewName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
