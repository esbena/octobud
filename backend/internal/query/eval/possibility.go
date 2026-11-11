package eval

import (
	"strings"

	"github.com/octobud-hq/octobud/backend/internal/db"
	"github.com/octobud-hq/octobud/backend/internal/query/parse"
)

type possibility uint8

const (
	uncertain possibility = iota
	impossible
	certain
)

// CouldMatch avoids dismissing a row based on unknown metadata in a referenced
// view. SQL-only fields such as tags must stay unknown even under negation.
func (e *Evaluator) CouldMatch(notif *db.Notification, repo *db.Repository) bool {
	if len(e.Views) == 0 {
		return e.Matches(notif, repo)
	}
	return e.possibleMatch(notif) != impossible
}

func (e *Evaluator) possibleMatch(notif *db.Notification) possibility {
	if e.ast == nil {
		return known(e.matchesInboxDefaults(notif))
	}
	if !parse.HasInOperator(e.ast) && notif.Muted && !parse.HasExplicitMuted(e.ast) {
		return impossible
	}
	return e.possibleNode(notif, e.ast)
}

func (e *Evaluator) possibleNode(notif *db.Notification, node parse.Node) possibility {
	switch n := node.(type) {
	case *parse.BinaryExpr:
		left, right := e.possibleNode(notif, n.Left), e.possibleNode(notif, n.Right)
		if n.Op == "AND" {
			if left == impossible || right == impossible {
				return impossible
			}
			if left == certain && right == certain {
				return certain
			}
			return uncertain
		}
		return possibleEither(left, right)
	case *parse.NotExpr:
		return possibleNot(e.possibleNode(notif, n.Expr))
	case *parse.ParenExpr:
		return e.possibleNode(notif, n.Expr)
	case *parse.Term:
		result := impossible
		for _, value := range n.Values {
			value = strings.ToLower(strings.TrimSpace(value))
			valueResult := uncertain
			switch strings.ToLower(n.Field) {
			case "in":
				if view, ok := e.Views[value]; ok {
					valueResult = view.possibleMatch(notif)
				} else if parse.IsBuiltinInValue(value) {
					valueResult = known(e.evaluateInCondition(notif, value))
				}
			case "is":
				valueResult = known(e.evaluateIsCondition(notif, value))
			}
			result = possibleEither(result, valueResult)
		}
		if n.Negated {
			return possibleNot(result)
		}
		return result
	default:
		return uncertain
	}
}

func known(matches bool) possibility {
	if matches {
		return certain
	}
	return impossible
}

func possibleNot(value possibility) possibility {
	switch value {
	case certain:
		return impossible
	case impossible:
		return certain
	default:
		return uncertain
	}
}

func possibleEither(left, right possibility) possibility {
	if left == certain || right == certain {
		return certain
	}
	if left == impossible && right == impossible {
		return impossible
	}
	return uncertain
}
