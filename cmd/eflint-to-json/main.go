package main

import (
	"encoding/json"
	"fmt"
	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
	"log"
	"os"
	"strconv"
	"strings"
)

var (
	eflintLexer = lexer.MustSimple([]lexer.SimpleRule{
		{"whitespace", `\s+`},
		{"Comment", `//.*`},
		{`FactID`, `[a-z][a-z_-]*`},
		{`Fact`, `Fact`},
		{`StringType`, `String`},
		{`IntType`, `Int`},

		{`IdentifiedBy`, `Identified by`},
		{`DerivedFrom`, `Derived from`},
		{`HoldsWhen`, `Holds when`},
		{`ConditionedBy`, `Conditioned by`},
		{`Placeholder`, `Placeholder`},
		{`Predicate`, `Predicate`},
		{`Invariant`, `Invariant`},
		{`Event`, `Event`},
		{`Duty`, `Duty`},
		{`RelatedTo`, `Related to`},
		{`SyncsWith`, `Syncs with`},
		{`Creates`, `Creates`},
		{`Terminates`, `Terminates`},
		{`Obfuscates`, `Obfuscates`},
		{`Actor`, `Actor`},
		{`Act`, `Act`},
		{`Recipient`, `Recipient`},
		{`Extend`, `Extend`},
		{`Holder`, `Holder`},
		{`Claimant`, `Claimant`},

		{`Foreach`, `Foreach`},
		{`For`, `For`},
		{`When`, `When`},

		// Iterators
		{`Count`, `Count`},
		{`Sum`, `Sum`},
		{`Max`, `Max`},
		{`Min`, `Min`},

		{`Not`, `Not`},

		{`True`, `True`},
		{`False`, `False`},
		{`OR`, `\|\|`},
		{`AND`, `&&`},
		{`EQ`, `==`},
		{`NEQ`, `!=`},
		{`GTE`, `>=`},
		{`LTE`, `<=`},
		{`GT`, `>`},
		{`LT`, `<`},
		{`NOT`, `NOT`},
		{`Neg`, `!`},
		{`Range`, `\.\.`},

		{`Int`, `[0-9]+`},
		{`String`, `[A-Z][a-z0-9]*`},

		// Statements
		{`Iquery`, `\?-`},
		{`Bquery`, `\?`},
		{`Create`, `\+`},
		{`Obfuscate`, `~`},
		{`Terminate`, `-`},

		{`Comma`, `,`},
		{`Star`, `\*`},
		{`Dot`, `\.`},
		{`Div`, `/`},
		{`Mod`, `%`},
		{`Range`, `\.\.`},
		{`LParen`, `\(`},
		{`RParen`, `\)`},
		{`Colon`, `:`},
		{"comment", `[#;][^\n]*`},
		{"Newline", `\n`},
	})
	parser = participle.MustBuild[Input](
		participle.Lexer(eflintLexer),
		participle.Union[Phrase](Fact{}, Query{}, Statement{}, Placeholder{}, Predicate{}, Event{}, Act{}, Duty{}, Extend{}),
		// TODO: Figure out how to deal with parentheses in expressions (e.g. "a && (b || c)").
		//participle.Union[Expression](ConstructorApplication{}, Reference{}, Operator{}, String{}, Int{}),
		participle.Union[Range](String{}, Int{}),
		participle.ParseTypeWith[Expression](parseExpression),
		participle.Elide("Comment"),
	)
	version = "0.1.0"
	kind    = "phrases"
	updates = true

	precedences = map[string]precedence{
		"||": {1, 1},
		"&&": {1, 1},
		"==": {2, 2},
		"!=": {2, 2},
		"+":  {3, 3},
		"-":  {3, 3},
		"*":  {5, 4},
		"/":  {7, 6},
		"%":  {9, 8},
		"<":  {10, 10},
		">":  {10, 10},
		"<=": {10, 10},
		">=": {10, 10},
	}

	operatorNames = map[string]string{
		"+": "ADD",
		"-": "SUB",
		"*": "MUL",
		"/": "DIV",
		"%": "MOD",

		"==": "EQ",
		"!=": "NEQ",

		"<":  "LT",
		">":  "GT",
		">=": "GTE",
		"<=": "LTE",

		"!": "NOT",

		"WHEN":  "WHEN",
		"SUM":   "SUM",
		"MAX":   "MAX",
		"MIN":   "MIN",
		"COUNT": "COUNT",

		"Holds": "HOLDS",
	}
)

type precedence struct{ Left, Right int }

type Input struct {
	Version string   `json:"version" parser:""`
	Kind    string   `json:"kind"    parser:""`
	Phrases []Phrase `json:"phrases" parser:"@@*"`
	Updates bool     `json:"updates" parser:""`
}

type Phrase interface {
	phrase()
}

type Range interface {
	isRange()
}

type Fact struct {
	Kind          string        `json:"kind"                     parser:""`
	Stateless     bool          `json:"stateless,omitempty"      parser:""`
	Updates       bool          `json:"updates,omitempty"        parser:""`
	Name          string        `json:"name,omitempty"           parser:"Fact @FactID"`
	Type          string        `json:"type,omitempty"           parser:"( (IdentifiedBy @(StringType | IntType))"`
	IdentifiedBy  []string      `json:"identified-by,omitempty"  parser:"| (IdentifiedBy @FactID ( Star @FactID )*)"`
	Range         []Range       `json:"range,omitempty"          parser:"| (IdentifiedBy (?= Int) @@ Range (?= Int) @@) | (IdentifiedBy @@ (Comma @@)*))?"`
	DerivedFrom   []Expression  `json:"derived-from,omitempty"   parser:"( (DerivedFrom @@ (Comma @@)*)"`
	HoldsWhen     []Expression  `json:"holds-when,omitempty"     parser:"| (HoldsWhen @@ (Comma @@)*)"`
	ConditionedBy []Expression  `json:"conditioned-by,omitempty" parser:"| (ConditionedBy @@ (Comma @@)*) )*"`
	Tokens        []lexer.Token `json:"-" parser:""`
}

func (f Fact) phrase() {}

type Query struct {
	Kind      string     `json:"kind"                     parser:"@(Iquery | Bquery)"`
	Stateless bool       `json:"stateless,omitempty"      parser:""`
	Updates   bool       `json:"updates,omitempty"        parser:""`
	Operand   Expression `json:"expression"               parser:"@@"`
}

func (q Query) phrase() {}

type Statement struct {
	Kind    string     `json:"kind"    parser:"@(Create | Obfuscate | Terminate)"`
	Operand Expression `json:"operand" parser:"@@"`
}

func (s Statement) phrase() {}

type Placeholder struct {
	Kind string   `json:"kind" parser:"Placeholder"`
	Name []string `json:"name" parser:"@FactID"`
	For  string   `json:"for"  parser:"For @FactID"`
}

func (p Placeholder) phrase() {}

type IsInvariant bool

func (b *IsInvariant) Capture(values []string) error {
	*b = values[0] == "Invariant"
	return nil
}

type Predicate struct {
	Kind        string      `json:"kind"                   parser:""`
	IsInvariant IsInvariant `json:"is-invariant,omitempty" parser:"@(Invariant | Predicate)"`
	Name        string      `json:"name"                   parser:"@FactID"`
	Expression  Expression  `json:"expression"             parser:"When @@"`
}

func (p Predicate) phrase() {}

type Event struct {
	Kind          string       `json:"kind"                     parser:"Event" default:"Event"`
	Name          string       `json:"name"                     parser:"@FactID"`
	RelatedTo     []string     `json:"related-to,omitempty"     parser:"(RelatedTo @FactID ( Comma @FactID )*)?"`
	DerivedFrom   []Expression `json:"derived-from,omitempty"   parser:"( (DerivedFrom   @@ (Comma @@)*)"`
	HoldsWhen     []Expression `json:"holds-when,omitempty"     parser:"| (HoldsWhen     @@ (Comma @@)*)"`
	ConditionedBy []Expression `json:"conditioned-by,omitempty" parser:"| (ConditionedBy @@ (Comma @@)*)"`
	SyncsWith     []Expression `json:"syncs-with,omitempty"     parser:"| (SyncsWith     @@ (Comma @@)*)"`
	Creates       []Expression `json:"creates,omitempty"        parser:"| (Creates       @@ (Comma @@)*)"`
	Terminates    []Expression `json:"terminates,omitempty"     parser:"| (Terminates    @@ (Comma @@)*)"`
	Obfuscates    []Expression `json:"obfuscates,omitempty"     parser:"| (Obfuscates    @@ (Comma @@)*) )*"`
}

func (e Event) phrase() {}

type Act struct {
	Kind          string       `json:"kind"                     parser:"Act" default:"Act"`
	Name          string       `json:"name"                     parser:"@FactID"`
	Actor         string       `json:"actor,omitempty"          parser:"Actor @FactID"`
	RelatedTo     []string     `json:"related-to,omitempty"     parser:"(RelatedTo @FactID ( Comma @FactID )*)?"`
	DerivedFrom   []Expression `json:"derived-from,omitempty"   parser:"( (DerivedFrom   @@ (Comma @@)*)"`
	HoldsWhen     []Expression `json:"holds-when,omitempty"     parser:"| (HoldsWhen     @@ (Comma @@)*)"`
	ConditionedBy []Expression `json:"conditioned-by,omitempty" parser:"| (ConditionedBy @@ (Comma @@)*)"`
	SyncsWith     []Expression `json:"syncs-with,omitempty"     parser:"| (SyncsWith     @@ (Comma @@)*)"`
	Creates       []Expression `json:"creates,omitempty"        parser:"| (Creates       @@ (Comma @@)*)"`
	Terminates    []Expression `json:"terminates,omitempty"     parser:"| (Terminates    @@ (Comma @@)*)"`
	Obfuscates    []Expression `json:"obfuscates,omitempty"     parser:"| (Obfuscates    @@ (Comma @@)*) )*"`
}

func (a Act) phrase() {}

type Duty struct {
	Kind          string       `json:"kind"                     parser:"Duty" default:"Duty"`
	Name          string       `json:"name"                     parser:"@FactID"`
	Holder        string       `json:"holder"                   parser:"Holder @FactID"`
	Claimant      string       `json:"claimant"                 parser:"Claimant @FactID"`
	RelatedTo     []string     `json:"related-to,omitempty"     parser:"(RelatedTo @FactID ( Comma @FactID )*)?"`
	DerivedFrom   []Expression `json:"derived-from,omitempty"   parser:"( (DerivedFrom   @@ (Comma @@)*)"`
	HoldsWhen     []Expression `json:"holds-when,omitempty"     parser:"| (HoldsWhen     @@ (Comma @@)*)"`
	ConditionedBy []Expression `json:"conditioned-by,omitempty" parser:"| (ConditionedBy @@ (Comma @@)*) )*"`
	ViolatedWhen  Expression   `json:"violated-when"            parser:"@@"`
}

func (d Duty) phrase() {}

type Extend struct {
	ParentKind string `json:"parent-kind" parser:"Extend @(Fact | Act | Event | Duty)"`
	Name       string `json:"name"        parser:"@FactID"`
	// TODO: Add support for extending
}

func (e Extend) phrase() {}

type Expression interface {
	expression()
}

func parseExpressionAtom(lex *lexer.PeekingLexer) (Expression, error) {
	switch peek := lex.Peek(); {
	case peek.Value == "Foreach" || peek.Value == "Exists":
		lex.Next()

		id := lex.Next()

		if id.Type != eflintLexer.Symbols()["FactID"] {
			return nil, participle.Errorf(id.Pos, "expected fact ID")
		}

		if lex.Next().Value != ":" {
			return nil, participle.Errorf(id.Pos, "expected :")
		}

		expr, err := parseExpression(lex)
		if err != nil {
			return nil, err
		}

		return Iterator{
			Iterator:   strings.ToUpper(peek.Value),
			Binds:      []string{id.Value},
			Expression: expr,
		}, nil
	case peek.Value == "Count" || peek.Value == "Sum" || peek.Value == "Min" || peek.Value == "Max" || peek.Value == "Holds":
		lex.Next()

		if lex.Peek().Value != "(" {
			return nil, participle.Errorf(lex.Peek().Pos, "expected (")
		}

		lex.Next()

		if peek.Value != "Holds" && lex.Peek().Value != "Foreach" {
			return nil, participle.Errorf(lex.Peek().Pos, "expected Foreach")
		}

		expr, err := parseExpression(lex)
		if err != nil {
			return nil, err
		}

		if lex.Peek().Value != ")" {
			return nil, participle.Errorf(lex.Peek().Pos, "expected )")
		}

		lex.Next()

		return Operator{
			Left:     expr,
			Operator: strings.ToUpper(peek.Value),
			Right:    nil,
		}, nil

	case peek.Type == eflintLexer.Symbols()["FactID"]:
		id := lex.Next()

		if lex.Peek().Type == eflintLexer.Symbols()["LParen"] {
			lex.Next()
			expr, err := parseExpression(lex)
			if err != nil {
				if lex.Peek().Type == eflintLexer.Symbols()["RParen"] {
					lex.Next()
					return ConstructorApplication{
						Identifier: id.Value,
						Operands:   []Expression{},
					}, nil
				}
				return nil, err
			}
			operands := []Expression{expr}

			for lex.Peek().Type == eflintLexer.Symbols()["Comma"] {
				lex.Next()
				expr, err := parseExpression(lex)
				if err != nil {
					return nil, err
				}
				operands = append(operands, expr)
			}

			if lex.Peek().Type != eflintLexer.Symbols()["RParen"] {
				return nil, participle.Errorf(lex.Next().Pos, "expected )")
			}
			lex.Next()
			return ConstructorApplication{
				Identifier: id.Value,
				Operands:   operands,
			}, nil
		}

		return Reference{id.Value}, nil
	case peek.Type == eflintLexer.Symbols()["String"]:
		return String{lex.Next().Value}, nil
	case peek.Type == eflintLexer.Symbols()["Int"]:
		val, err := strconv.ParseInt(lex.Next().Value, 10, 64)
		if err != nil {
			return nil, err
		}
		return Int{val}, nil
	case peek.Value == "(":
		lex.Next()
		expr, err := parseExpression(lex)
		if err != nil {
			return nil, err
		}
		if lex.Peek().Value != ")" {
			return nil, participle.Errorf(lex.Next().Pos, "expected )")
		}
		lex.Next()
		return expr, nil
	case peek.Value == "!":
		log.Println("!")
		lex.Next()
		expr, err := parseExpressionAtom(lex)
		if err != nil {
			return nil, err
		}
		return Operator{
			Left:     expr,
			Operator: "!",
			Right:    nil,
		}, nil
	default:
		return nil, participle.NextMatch
	}
}

func parseExpressionPrec(lex *lexer.PeekingLexer, minPrec int) (Expression, error) {
	lhs, err := parseExpressionAtom(lex)
	if err != nil {
		return nil, err
	}
	//log.Println("lhs", lhs)

	for {
		peek := lex.Peek()
		prec, ok := precedences[peek.Value]
		if !ok || prec.Left < minPrec {
			break
		}
		op := lex.Next().Value
		rhs, err := parseExpressionPrec(lex, prec.Right)
		//log.Println("rhs", rhs)
		if err != nil {
			return nil, err
		}
		lhs = Operator{lhs, op, rhs}
	}

	return lhs, nil
}

func parseExpression(lex *lexer.PeekingLexer) (Expression, error) {
	expr, err := parseExpressionPrec(lex, 0)

	if err != nil {
		return nil, err
	}

	if lex.Peek().Value == "." {
		lex.Next()
	} else if lex.Peek().Value == "When" {
		lex.Next()
		rhs, err := parseExpression(lex)
		if err != nil {
			return nil, err
		}
		return Operator{
			Left:     expr,
			Operator: "WHEN",
			Right:    rhs,
		}, nil
	}

	return expr, nil
}

type Iterator struct { // TODO: Only a foreach can be inside an iterator
	Iterator   string     `json:"iterator"`
	Binds      []string   `json:"binds"`
	Expression Expression `json:"expression"`
}

func (i Iterator) expression() {}

type Not struct {
	Expression Expression `json:"expression" parser:"Not @@"`
}

func (n Not) expression() {}

//func (n Not) MarshalJSON() ([]byte, error) {
//	return json.Marshal(Operator{
//		Operator: "NOT",
//		Operands: []Expression{n.Expression},
//	})
//}

type String struct {
	Value string `parser:"@String"`
}

func (s String) expression() {}
func (s String) isRange()    {}

func (s String) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Value)
}

type Int struct {
	Value int64 `parser:"@Int"`
}

func (i Int) expression() {}
func (i Int) isRange()    {}

func (i Int) MarshalJSON() ([]byte, error) {
	return json.Marshal(i.Value)
}

type Reference struct {
	Value string `parser:"@FactID"`
}

func (r Reference) expression() {}

func (r Reference) MarshalJSON() ([]byte, error) {
	return json.Marshal([]string{r.Value})
}

type ConstructorApplication struct {
	Identifier string       `json:"identifier" parser:"@FactID"`
	Operands   []Expression `json:"operands"   parser:"( LParen (@@ (Comma @@)*)? RParen )+"`
}

func (c ConstructorApplication) expression() {}

type Operator struct {
	Left     Expression `json:"left"`
	Operator string     `json:"operator"`
	Right    Expression `json:"right"`
}

func (o Operator) expression() {}

func (o Operator) MarshalJSON() ([]byte, error) {
	Operands := make([]Expression, 0, 2)

	if o.Left != nil {
		Operands = append(Operands, o.Left)
	}

	if o.Right != nil {
		Operands = append(Operands, o.Right)
	}

	return json.Marshal(struct {
		Operator string       `json:"operator"`
		Operands []Expression `json:"operands"`
	}{
		Operator: operatorNames[o.Operator],
		Operands: Operands,
	})

}

func parseRangeValues(r []Range, tokens []lexer.Token) ([]Range, error) {
	for t := range tokens {
		if tokens[t].Value == ".." {
			if len(r) != 2 {
				return nil, fmt.Errorf("invalid range")
			}
			if _, ok := r[0].(Int); !ok {
				return nil, fmt.Errorf("invalid range")
			}
			if _, ok := r[1].(Int); !ok {
				return nil, fmt.Errorf("invalid range")
			}
			lower, upper := r[0].(Int).Value, r[1].(Int).Value

			if lower > upper {
				return nil, fmt.Errorf("invalid range")
			}

			r = []Range{Int{lower}}
			for i := lower + 1; i <= upper; i++ {
				r = append(r, Int{i})
			}
		}
	}

	return r, nil
}

func parseRangeType(r []Range) (string, bool) {
	rangeType := ""

	for _, e := range r {
		switch e.(type) {
		case String:
			if rangeType == "" {
				rangeType = "String"
			} else if rangeType != "String" {
				return rangeType, false
			}
		case Int:
			if rangeType == "" {
				rangeType = "Int"
			} else if rangeType != "Int" {
				return rangeType, false
			}
		}
	}

	return rangeType, true
}

func main() {
	//fmt.Println(parser.String())
	filename := ""
	file := os.Stdin
	if len(os.Args) > 1 {
		f, err := os.Open(os.Args[1])
		if err != nil {
			panic(err)
		}
		defer f.Close()
		filename = os.Args[1]
		file = f
	}

	ini, err := parser.Parse(filename, file)
	if err != nil {
		panic(err)
	}
	//pp.Println(ini)

	// Add metadata
	ini.Version = version
	ini.Kind = kind
	ini.Updates = updates

	// Fill in missing fields
	for i, phrase := range (*ini).Phrases {
		switch phrase.(type) {
		case Fact:
			f := phrase.(Fact)
			if len(f.IdentifiedBy) > 0 {
				// Composite fact
				f.Kind = "cfact"
			} else {
				// Atomic fact
				f.Kind = "afact"

				if f.Type == "" {
					if len(f.Range) > 0 {
						rangeType, ok := parseRangeType(f.Range)
						if !ok {
							panic("range type mismatch")
						}
						f.Type = rangeType
						rangeValues, err := parseRangeValues(f.Range, f.Tokens)
						if err != nil {
							panic(err)
						}
						f.Range = rangeValues
					} else {
						f.Type = "String"
					}
				}
			}

			ini.Phrases[i] = f
		case Query:
			q := phrase.(Query)
			if q.Kind == "?" {
				q.Kind = "bquery"
			} else if q.Kind == "?-" {
				q.Kind = "iquery"
			} else {
				panic("unknown query type")
			}
			ini.Phrases[i] = q
		case Statement:
			s := phrase.(Statement)
			if s.Kind == "+" {
				s.Kind = "create"
			} else if s.Kind == "-" {
				s.Kind = "terminate"
			} else if s.Kind == "~" {
				s.Kind = "obfuscate"
			} else {
				panic("unknown statement type")
			}
			ini.Phrases[i] = s
		case Placeholder:
			p := phrase.(Placeholder)
			p.Kind = "placeholder"
			ini.Phrases[i] = p
		case Predicate:
			p := phrase.(Predicate)
			p.Kind = "predicate"
			ini.Phrases[i] = p
		}
	}

	// Encode it and output it as JSON
	json, err := json.MarshalIndent(ini, "", "  ")
	if err != nil {
		panic(err)
	}

	os.Stdout.Write(json)
}
