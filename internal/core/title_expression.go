package core

import (
	"fmt"
	"strings"
	"unicode"
)

// TitleExpression 是已解析的标题布尔表达式。
type TitleExpression struct {
	root titleExpressionNode
}

type titleExpressionNode interface {
	match(string) bool
}

type titleTerm string

func (term titleTerm) match(title string) bool {
	return strings.Contains(title, string(term))
}

type titleNot struct{ child titleExpressionNode }

func (node titleNot) match(title string) bool { return !node.child.match(title) }

type titleAnd struct{ left, right titleExpressionNode }

func (node titleAnd) match(title string) bool {
	return node.left.match(title) && node.right.match(title)
}

type titleOr struct{ left, right titleExpressionNode }

func (node titleOr) match(title string) bool {
	return node.left.match(title) || node.right.match(title)
}

type titleExpressionParser struct {
	runes []rune
	pos   int
}

// ParseTitleExpression 解析支持 !、&、|、括号和双引号关键词的标题表达式。
func ParseTitleExpression(value string) (TitleExpression, error) {
	if strings.TrimSpace(value) == "" {
		return TitleExpression{}, nil
	}
	parser := &titleExpressionParser{runes: []rune(value)}
	root, err := parser.parseOr()
	if err != nil {
		return TitleExpression{}, err
	}
	parser.skipSpace()
	if parser.pos != len(parser.runes) {
		return TitleExpression{}, parser.errorf("unexpected %q", parser.runes[parser.pos])
	}
	return TitleExpression{root: root}, nil
}

// Match 判断主标题是否满足表达式，关键词按 Unicode 小写后做子串匹配。
func (expression TitleExpression) Match(title string) bool {
	if expression.root == nil {
		return true
	}
	return expression.root.match(strings.ToLower(title))
}

func (parser *titleExpressionParser) parseOr() (titleExpressionNode, error) {
	left, err := parser.parseAnd()
	if err != nil {
		return nil, err
	}
	for {
		parser.skipSpace()
		if !parser.consume('|') {
			return left, nil
		}
		right, err := parser.parseAnd()
		if err != nil {
			return nil, err
		}
		left = titleOr{left: left, right: right}
	}
}

func (parser *titleExpressionParser) parseAnd() (titleExpressionNode, error) {
	left, err := parser.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		parser.skipSpace()
		if !parser.consume('&') {
			return left, nil
		}
		right, err := parser.parseUnary()
		if err != nil {
			return nil, err
		}
		left = titleAnd{left: left, right: right}
	}
}

func (parser *titleExpressionParser) parseUnary() (titleExpressionNode, error) {
	parser.skipSpace()
	if parser.consume('!') {
		child, err := parser.parseUnary()
		if err != nil {
			return nil, err
		}
		return titleNot{child: child}, nil
	}
	return parser.parsePrimary()
}

func (parser *titleExpressionParser) parsePrimary() (titleExpressionNode, error) {
	parser.skipSpace()
	if parser.pos >= len(parser.runes) {
		return nil, parser.errorf("expected keyword")
	}
	if parser.consume('(') {
		node, err := parser.parseOr()
		if err != nil {
			return nil, err
		}
		parser.skipSpace()
		if !parser.consume(')') {
			return nil, parser.errorf("expected closing parenthesis")
		}
		return node, nil
	}
	if parser.runes[parser.pos] == '"' {
		return parser.parseQuotedTerm()
	}
	return parser.parseBareTerm()
}

func (parser *titleExpressionParser) parseQuotedTerm() (titleExpressionNode, error) {
	parser.pos++
	value := make([]rune, 0)
	for parser.pos < len(parser.runes) {
		current := parser.runes[parser.pos]
		parser.pos++
		if current == '"' {
			if len(value) == 0 {
				return nil, parser.errorf("keyword cannot be empty")
			}
			return titleTerm(strings.ToLower(string(value))), nil
		}
		if current == '\\' {
			if parser.pos >= len(parser.runes) {
				return nil, parser.errorf("unfinished escape")
			}
			current = parser.runes[parser.pos]
			parser.pos++
			if current != '\\' && current != '"' {
				return nil, parser.errorf("only quote and backslash can be escaped")
			}
		}
		value = append(value, current)
	}
	return nil, parser.errorf("unterminated quoted keyword")
}

func (parser *titleExpressionParser) parseBareTerm() (titleExpressionNode, error) {
	start := parser.pos
	for parser.pos < len(parser.runes) && !strings.ContainsRune("!&|()", parser.runes[parser.pos]) {
		parser.pos++
	}
	value := strings.TrimFunc(string(parser.runes[start:parser.pos]), unicode.IsSpace)
	if value == "" {
		return nil, parser.errorf("expected keyword")
	}
	return titleTerm(strings.ToLower(value)), nil
}

func (parser *titleExpressionParser) skipSpace() {
	for parser.pos < len(parser.runes) && unicode.IsSpace(parser.runes[parser.pos]) {
		parser.pos++
	}
}

func (parser *titleExpressionParser) consume(expected rune) bool {
	if parser.pos < len(parser.runes) && parser.runes[parser.pos] == expected {
		parser.pos++
		return true
	}
	return false
}

func (parser *titleExpressionParser) errorf(format string, args ...any) error {
	return fmt.Errorf("position %d: %s", parser.pos+1, fmt.Sprintf(format, args...))
}
